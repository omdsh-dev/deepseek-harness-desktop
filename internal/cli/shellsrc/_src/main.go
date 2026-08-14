// Package shell 是打包后的桌面应用壳（Wails v3 原生窗口）。
//
// 壳是应用的唯一入口，同时是 SEA 后端的守护进程：启动同目录的 dsh-server
// （`dsh --profile <name> --port 0`，端口由 OS 分配避免冲突），从后端
// stdout 解析实际监听地址，用 Wails 的 WebviewWindow 内嵌加载。后端异常
// 退出时自动退避重启并重新指向新地址。应用退出（窗口关闭）时终止后端
// 进程组，不留孤儿 node。
//
// 壳由 CLI 在打包时构建，构建前 CLI 在壳可执行文件同目录写入 appconfig
// .json（应用名、窗口几何、profile 名、DSH_HOME 策略）——壳启动时读取，
// 缺省回退到与旧版一致的行为（1280x800、profile web、DSH_HOME 继承环境）。
package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed landing.html
var loadingHTML string

func main() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("resolve executable: %v", err)
	}
	exeDir := filepath.Dir(exe)

	cfg := loadAppConfig(exeDir)

	// 工作目录：DSH_APP_WORKSPACE（受限/测试环境可覆盖），默认用户主目录。
	workspace := os.Getenv("DSH_APP_WORKSPACE")
	if workspace == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("resolve home: %v", err)
		}
		workspace = home
	}
	// 后端监听端口：默认 "0" 由 OS 分配，避免冲突。
	port := os.Getenv("DSH_APP_PORT")
	if port == "" {
		port = "0"
	}
	if err := os.Chdir(workspace); err != nil {
		log.Fatalf("chdir %s: %v", workspace, err)
	}

	// DSH_HOME：按 appconfig 策略解析（copy / 固定路径 / env），
	// DSH_APP_DSH_HOME 环境变量可显式覆盖。
	dshHome, err := resolveDSHHome(cfg, exeDir)
	if err != nil {
		log.Fatalf("resolve DSH_HOME: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := application.New(application.Options{
		Name:        cfg.Name,
		Description: cfg.Name + " Desktop",
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// 收口（幂等，信号与窗口关闭共用）：cancel() 让 supervise 开始终止后端
	// 进程组，再 Quit() 让 Wails 事件循环退出，app.Run() 返回后 main 走
	// <-done 等 supervise 收口完成才真正退出。
	var quitOnce sync.Once
	quit := func(reason string) {
		quitOnce.Do(func() {
			log.Printf("%s，退出中", reason)
			cancel()
			app.Quit()
		})
	}

	// 信号处理：SIGTERM/SIGINT 不能直接终止壳进程——Go 默认对未捕获信号
	// 立即退出，defer cancel() 与 supervise 的进程终止都没机会执行，后端会
	// 残留为孤儿。捕获后走收口路径。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		quit(fmt.Sprintf("收到信号 %v", sig))
	}()

	// 窗口创建延迟到 app.Run()；守护 goroutine 会先 SetURL，窗口即以最新
	// 地址创建。HTML 是启动页（替代 Wails 默认空白页），就绪后由守护进程
	// 用 SetURL 切到真实地址。
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     cfg.Name,
		Width:     cfg.Window.Width,
		Height:    cfg.Window.Height,
		MinWidth:  cfg.Window.MinWidth,
		MinHeight: cfg.Window.MinHeight,
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBarDefault,
		},
		// Linux：显式钉死 WebKitGTK 硬件加速为 Always（渲染走 GPU 合成，
		// 避免 CPU 合成带来的内存/CPU 开销）。
		Linux: application.LinuxWindow{
			WebviewGpuPolicy: application.WebviewGpuPolicyAlways,
		},
		HTML: loadingHTML,
	})

	win.OnWindowEvent(events.Mac.WindowShouldClose, func(*application.WindowEvent) {
		quit("窗口关闭")
	})
	win.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		quit("窗口已关闭")
	})

	// 守护后端：启动、就绪、重启都由 supervise 负责。退出时先 cancel 让
	// supervise 终止后端进程组，再等它收口（done）——main 不能抢先返回，
	// 否则 Go 进程退出会强杀 goroutine，kill 来不及执行，后端残留为孤儿。
	done := make(chan struct{})
	go func() {
		defer close(done)
		supervise(ctx, exeDir, cfg.Profile, port, dshHome, win)
	}()

	// 信号若在 app.Run() 之前到达（启动瞬间被 kill）：cancel() 已让 supervise
	// 收口并返回，这里无需再进入 Wails 事件循环，直接等收口后退出。
	select {
	case <-ctx.Done():
		<-done
		return
	default:
	}

	if err := app.Run(); err != nil {
		cancel()
		<-done
		log.Fatalf("run app: %v", err)
	}
	cancel()
	<-done
}
