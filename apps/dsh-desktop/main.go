// dsh-desktop — 把 dsh 封装成不依赖外部浏览器的 macOS 桌面应用。
//
// 壳进程（本程序）是 DSH.app 的唯一入口，同时是 SEA 后端的守护进程：
// 它启动同目录下的 dsh-server（即 `dsh web --port 0`，端口由 OS 分配避免
// 冲突），从后端 stdout 解析实际监听地址，用 Wails 的 WebviewWindow 内嵌
// 加载。后端异常退出（网络/加载失败等）时由 supervise 自动退避重启并
// 重新指向新地址，全程不打开系统浏览器。应用退出（窗口关闭）时终止后端
// 进程组，不留孤儿 node（dsh-server 是内嵌 node 的 SEA 可执行）。
// 后端进程生命周期见 server 子包，守护循环见 supervise.go。
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

	// 加载前读取环境变量：全部在创建窗口/启动后端之前解析。
	// DSH_APP_WORKSPACE — 工作目录（默认用户主目录；受限/测试环境可覆盖）。
	// DSH_APP_PORT — 后端监听端口（默认 "0" 由 OS 分配，避免冲突）。
	workspace := os.Getenv("DSH_APP_WORKSPACE")
	if workspace == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("resolve home: %v", err)
		}
		workspace = home
	}
	port := os.Getenv("DSH_APP_PORT")
	if port == "" {
		port = "0"
	}
	if err := os.Chdir(workspace); err != nil {
		log.Fatalf("chdir %s: %v", workspace, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := application.New(application.Options{
		Name:        "DeepSeek Harness",
		Description: "DeepSeek Harness Desktop",
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

	// 信号处理：SIGTERM/SIGINT（外部 kill、终端 Ctrl+C 等）不能直接终止壳进程
	// —— Go 默认对未捕获信号的处理是立即退出，defer cancel() 与 supervise 的
	// 进程终止都没机会执行，dsh-server 会残留为孤儿。这里捕获后走收口路径。
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
		Title:     "DeepSeek Harness Desktop",
		Width:     1280,
		Height:    800,
		MinWidth:  800,
		MinHeight: 600,
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBarDefault,
		},
		HTML: loadingHTML,
	})

	// 窗口关闭（用户点左上角关闭按钮）：Wails 的
	// ApplicationShouldTerminateAfterLastWindowClosed 依赖 NSApplication 的
	// shouldTerminateAfterLastWindowClosed 委托链路，实测在部分环境不生效
	// （窗口已关闭但主线程仍停在事件循环）。这里直接监听关闭事件走收口，
	// 不依赖 Wails 的自动 terminate。
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
		supervise(ctx, exeDir, port, win)
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
