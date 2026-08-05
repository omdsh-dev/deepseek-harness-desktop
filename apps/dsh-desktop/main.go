// dsh-desktop — 把 dsh 封装成不依赖外部浏览器的 macOS 桌面应用。
//
// 壳进程（本程序）是 DSH.app 的唯一入口，同时是 SEA 后端的守护进程：
// 它启动同目录下的 dsh-server（即 `dsh web --port 0`，端口由 OS 分配避免
// 冲突），从后端 stdout 解析实际监听地址，用 Wails 的 WebviewWindow 内嵌
// 加载。后端异常退出（网络/加载失败等）时自动退避重启并重新指向新地址，
// 全程不打开系统浏览器。应用退出（窗口关闭）时终止后端进程组，不留孤儿
// node（dsh-server 是内嵌 node 的 SEA 可执行）。
package main

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed landing.html
var loadingHTML string

// DSH_SERVER_NAME 是 .app 里 SEA 后端可执行文件名（与壳同目录）。
const DSH_SERVER_NAME = "dsh-server"

// 后端 URL 就绪的等待上限，以及重启退避的初值与上限。
const (
	urlTimeout     = 30 * time.Second
	restartBackoff = time.Second
	maxRestartWait = 30 * time.Second
)

// serverStopGrace 是退出/超时时 SIGTERM 进程组后等待其退出的宽限期，
// 到期未退出则 SIGKILL 兜底。
const serverStopGrace = 5 * time.Second

// serverExit 报告一次后端进程的终结；区分正常退出与失败。
type serverExit struct {
	err error // 非 nil 表示非零退出或异常终结
}

// rcFileFor 按用户 shell 返回要 source 的配置文件路径（不检查存在性，
// 缺失时 source 报错但被重定向吞掉，不影响后续）。
func rcFileFor(shell string) string {
	switch filepath.Base(shell) {
	case "bash":
		return "~/.bashrc"
	case "zsh":
		return "~/.zshrc"
	default:
		return ""
	}
}

// shellQuote 用单引号包裹字符串，供拼进 shell 命令行（路径可能含空格）。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// serverCommand 构造 spawn dsh-server 的命令。当用户 shell 有对应 rc 文件时，
// 先 source 该文件（让后端继承用户终端里的环境变量，如 API key），再 exec
// dsh-server —— exec 保持同一进程（PID 不变），守护 wait 语义不受影响。
// source 的输出重定向到 /dev/null，避免污染后端 stdout 的 URL 行。
// 进程放入独立进程组（Setpgid）：应用退出时按组终止，保证后端
// （SEA 内嵌 node 的 dsh-server，及其将来可能 spawn 的子进程）整体清理，
// 不残留孤儿 node。ctx 取消不依赖 exec.CommandContext 的异步 kill（它只杀
// 直接子进程且时机不受控），由调用方经 stopServer 显式终止。
func serverCommand(exeDir, port string) *exec.Cmd {
	server := filepath.Join(exeDir, DSH_SERVER_NAME)
	args := []string{"web", "--port", port}

	shell := os.Getenv("SHELL")
	rc := rcFileFor(shell)
	var cmd *exec.Cmd
	if shell != "" && rc != "" {
		cmdline := fmt.Sprintf("source %s >/dev/null 2>&1; exec %s %s",
			rc, shellQuote(server), strings.Join(args, " "))
		cmd = exec.Command(shell, "-c", cmdline)
	} else {
		cmd = exec.Command(server, args...)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// killServer 向 dsh-server 所在进程组发信号（负 PID 覆盖组内全部进程）。
// 先 SIGTERM 给后端优雅退出机会；未及时退出由 stopServer 的 SIGKILL 兜底。
func killServer(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, sig)
}

// stopServer 终止后端并等待其退出：SIGTERM 整个进程组，serverStopGrace
// 内未退出则 SIGKILL 兜底，随后等待 Wait 收口（exitCh 必然收到结果）。
func stopServer(cmd *exec.Cmd, exitCh <-chan serverExit) {
	if cmd.Process == nil {
		return
	}
	killServer(cmd, syscall.SIGTERM)
	select {
	case <-exitCh:
		return
	case <-time.After(serverStopGrace):
	}
	killServer(cmd, syscall.SIGKILL)
	select {
	case <-exitCh:
	case <-time.After(serverStopGrace):
	}
}

// startServer 启动一次 SEA 后端（dsh-server web --port <port>），等待其把
// `dsh web: http://127.0.0.1:<port>` 打到 stdout，返回监听 URL。
// port 传 "0" 时由 OS 分配（默认，避免与已占用端口冲突）。ctx 取消或超时
// 时终止后端。返回 cmd 供调用方在退出/重启时终止进程组，exitCh 在进程
// 退出后收到终结结果。
func startServer(ctx context.Context, exeDir, port string) (cmd *exec.Cmd, url string, exitCh <-chan serverExit, err error) {
	cmd = serverCommand(exeDir, port)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return cmd, "", nil, fmt.Errorf("stdout pipe: %w", err)
	}
	// 后端 stderr 直接透传到壳的 stderr，便于诊断。
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return cmd, "", nil, fmt.Errorf("start %s: %w", DSH_SERVER_NAME, err)
	}

	// 进程终结信号：Wait 必须在读取完管道后调用，这里独立收口。
	exitChRaw := make(chan serverExit, 1)
	go func() {
		err := cmd.Wait()
		exitChRaw <- serverExit{err: err}
	}()

	// 逐行读 stdout，直到出现 URL 行；之后继续读到 EOF，防止后端持续写
	// stdout（agent 运行日志等）时管道缓冲填满而阻塞在 write —— 阻塞会拖住
	// 后端的优雅退出（SIGTERM handler 里 dispose 也写 stdout），进而拖住
	// stopServer 的收口。排空到 EOF 也满足 exec.Cmd.Wait 要求先读完管道。
	urlCh := make(chan string, 1)
	go func() {
		defer close(urlCh)
		reader := bufio.NewReader(stdout)
		urlSent := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return // EOF（后端退出）或读错误
			}
			if !urlSent && strings.HasPrefix(line, "dsh web: ") {
				urlCh <- strings.TrimSpace(strings.TrimPrefix(line, "dsh web: "))
				urlSent = true
			}
		}
	}()

	select {
	case u := <-urlCh:
		if u == "" {
			stopServer(cmd, exitChRaw)
			return cmd, "", exitChRaw, fmt.Errorf("server exited without publishing a URL")
		}
		return cmd, u, exitChRaw, nil
	case <-time.After(urlTimeout):
		stopServer(cmd, exitChRaw)
		return cmd, "", exitChRaw, fmt.Errorf("timed out waiting for dsh web URL")
	case <-ctx.Done():
		stopServer(cmd, exitChRaw)
		return cmd, "", exitChRaw, ctx.Err()
	}
}

// supervise 守护后端：启动 → 就绪后把窗口指向其 URL → 进程退出则退避重启，
// 直到 ctx 取消（应用退出）。后端在任意时刻意外终结都会走同一重启路径。
func supervise(ctx context.Context, exeDir, port string, win *application.WebviewWindow) {
	backoff := restartBackoff
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cmd, url, exitCh, err := startServer(ctx, exeDir, port)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("dsh server 启动失败：%v（%s 后重试）", err, backoff)
		} else {
			backoff = restartBackoff
			log.Printf("dsh server ready at %s", url)
			win.SetURL(url)

			// 等待本次进程终结（或应用退出）。
			select {
			case <-ctx.Done():
				// 应用退出：终止后端进程组并等待收口，不留孤儿 node。
				stopServer(cmd, exitCh)
				return
			case exit := <-exitCh:
				if exit.err != nil {
					log.Printf("dsh server 异常退出：%v", exit.err)
				} else {
					log.Printf("dsh server 退出（重启）")
				}
			}
		}

		// 退避等待后重启；应用退出则立即结束。
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < maxRestartWait {
			backoff *= 2
			if backoff > maxRestartWait {
				backoff = maxRestartWait
			}
		}
	}
}

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
	// stopServer 都没机会执行，dsh-server 会残留为孤儿。这里捕获后走收口路径。
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
