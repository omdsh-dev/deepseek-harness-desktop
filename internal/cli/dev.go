package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
)

// runWeb 启动 dsh web（DSH_HOME=homeDir），解析就绪 URL 后返回并保持
// 前台运行（dsh 的 stdout/stderr 透传，Ctrl+C 退出）。dsh web 就绪行：
// `dsh web: http://127.0.0.1:<port>`。
func runWeb(dshBin, homeDir string) (string, error) {
	cmd := exec.Command(dshBin, "web", "--port", "0")
	cmd.Env = append(os.Environ(), "DSH_HOME="+homeDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start dsh web: %w", err)
	}

	// 信号兜底：Ctrl+C / kill 时主动终止 dsh web，保证不留孤儿后端。
	// （终端 Ctrl+C 会把 SIGINT 发给整个前台进程组，dsh 同组也能收到；
	// 此处再兜底 CLI 单独收到信号的情况。）
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\n收到 %v，停止 dsh web\n", sig)
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		os.Exit(130)
	}()

	urlCh := make(chan string, 1)
	go func() {
		defer close(urlCh)
		reader := bufio.NewReader(stdout)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return // EOF（dsh 退出）或读错误
			}
			fmt.Print(line)
			if strings.HasPrefix(line, "dsh web: ") {
				urlCh <- strings.TrimSpace(strings.TrimPrefix(line, "dsh web: "))
			}
		}
	}()

	select {
	case u := <-urlCh:
		if u == "" {
			cmd.Wait()
			return "", fmt.Errorf("dsh web 未输出就绪 URL 即退出")
		}
		// 前台保持：阻塞等待 dsh web 退出（Ctrl+C 发给整个前台进程组，
		// CLI 与 dsh 一起退出），不让后端残留为孤儿进程。
		return u, cmd.Wait()
	case err := <-waitErr(cmd):
		return "", fmt.Errorf("dsh web 退出: %w", err)
	}
}

func waitErr(cmd *exec.Cmd) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- cmd.Wait() }()
	return ch
}

// openURL 用系统默认浏览器打开 URL（平台命令）。
func openURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
