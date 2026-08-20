// Package main 是打包后的桌面应用壳入口（Wails v3 原生窗口）。
//
// 壳是应用的唯一入口，同时是 SEA 后端的守护进程：启动同目录的 dsh-server
// （`dsh --profile <name> --port 0`，端口由 OS 分配避免冲突），从后端
// stdout 解析实际监听地址，用 WebviewWindow 内嵌加载；后端异常退出时退避
// 重启，应用退出时终止后端进程组，不留孤儿 node。配置、DSH_HOME、后端守护
// 分别由 appconfig / dshhome / supervise 子包实现，入口只做装配。
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

	"github.com/omdsh-dev/dsh-web-desktopify/pkg/shell/appconfig"
	"github.com/omdsh-dev/dsh-web-desktopify/pkg/shell/dshhome"
	"github.com/omdsh-dev/dsh-web-desktopify/pkg/shell/supervise"
)

//go:embed landing.html
var loadingHTML string

func main() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("resolve executable: %v", err)
	}
	exeDir := filepath.Dir(exe)

	cfg := appconfig.Load(exeDir)

	// 工作目录默认用户主目录，DSH_APP_WORKSPACE 可覆盖（受限/测试环境）。
	workspace := os.Getenv("DSH_APP_WORKSPACE")
	if workspace == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("resolve home: %v", err)
		}
		workspace = home
	}
	// 后端端口默认 "0" 由 OS 分配，避免冲突。
	port := os.Getenv("DSH_APP_PORT")
	if port == "" {
		port = "0"
	}
	if err := os.Chdir(workspace); err != nil {
		log.Fatalf("chdir %s: %v", workspace, err)
	}

	dshHome, err := dshhome.Resolve(cfg, exeDir)
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

	// 收口（幂等，信号与窗口关闭共用）：cancel() 让 supervise 终止后端进程组，
	// 再 Quit() 退出 Wails 事件循环；app.Run() 返回后等 supervise 收口完成。
	var quitOnce sync.Once
	quit := func(reason string) {
		quitOnce.Do(func() {
			log.Printf("%s，退出中", reason)
			cancel()
			app.Quit()
		})
	}

	// 捕获 SIGTERM/SIGINT 走收口路径：Go 默认对未捕获信号立即退出，defer
	// cancel() 与 supervise 的进程终止都没机会执行，后端会残留为孤儿。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		quit(fmt.Sprintf("收到信号 %v", sig))
	}()

	// 窗口创建延迟到 app.Run()；守护 goroutine 会先 SetURL，窗口即以最新地址
	// 创建。HTML 是启动页，就绪后由守护进程切到真实地址。
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     cfg.Name,
		Width:     cfg.Window.Width,
		Height:    cfg.Window.Height,
		MinWidth:  cfg.Window.MinWidth,
		MinHeight: cfg.Window.MinHeight,
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBarDefault,
		},
		// Linux：钉死 WebKitGTK 硬件加速为 Always，避免 CPU 合成开销。
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

	// 退出时先 cancel 让 supervise 终止后端进程组，再等它收口（done）——main
	// 不能抢先返回，否则 Go 进程退出会强杀 goroutine，kill 来不及执行。
	done := make(chan struct{})
	go func() {
		defer close(done)
		supervise.Run(ctx, exeDir, cfg.Profile, port, dshHome, win)
	}()

	// 信号若在 app.Run() 之前到达（启动瞬间被 kill）：supervise 已收口，直接退出。
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
