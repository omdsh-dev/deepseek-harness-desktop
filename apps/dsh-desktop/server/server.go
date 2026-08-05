// Package server 管理 SEA 后端（dsh-server）进程的生命周期：构造启动命令、
// 等待就绪 URL、终止进程组。不依赖 Wails，可独立测试。
//
// dsh-server 是 DSH.app 同目录下的 SEA 可执行（内嵌 node 的 `dsh web`），
// 启动参数 `web --port <port>`（端口传 "0" 由 OS 分配，避免冲突）。就绪后
// 它把 `dsh web: http://127.0.0.1:<port>` 打到 stdout，本包解析该行得到
// 实际监听地址。进程放入独立进程组（Setpgid）：应用退出时按组终止，保证
// 后端（及将来可能 spawn 的子进程）整体清理，不留孤儿 node。
package server

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Name 是 .app 里 SEA 后端可执行文件名（与壳同目录）。
const Name = "dsh-server"

// readyPrefix 是后端就绪行前缀；壳从此解析实际监听 URL。
const readyPrefix = "dsh web: "

// 后端 URL 就绪的等待上限，以及 SIGTERM 后等待退出的宽限期
// （到期未退出则 SIGKILL 兜底）。
const (
	urlTimeout = 30 * time.Second
	stopGrace  = 5 * time.Second
)

// Exit 报告一次后端进程的终结；区分正常退出与失败。
type Exit struct {
	Err error // 非 nil 表示非零退出或异常终结
}

// Process 是一次 dsh-server 进程的生命周期句柄。
type Process struct {
	cmd    *exec.Cmd
	exitCh <-chan Exit
}

// Exit 返回进程终结结果；进程退出后收到。
func (p *Process) Exit() <-chan Exit {
	return p.exitCh
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

// command 构造 spawn dsh-server 的命令。当用户 shell 有对应 rc 文件时，
// 先 source 该文件（让后端继承用户终端里的环境变量，如 API key），再 exec
// dsh-server —— exec 保持同一进程（PID 不变），守护 wait 语义不受影响。
// source 的输出重定向到 /dev/null，避免污染后端 stdout 的 URL 行。
// 进程放入独立进程组（Setpgid）：应用退出时按组终止，保证后端
// （SEA 内嵌 node 的 dsh-server，及其将来可能 spawn 的子进程）整体清理，
// 不残留孤儿 node。ctx 取消不依赖 exec.CommandContext 的异步 kill（它只杀
// 直接子进程且时机不受控），由调用方经 Process.Stop 显式终止。
func command(exeDir, port string) *exec.Cmd {
	server := filepath.Join(exeDir, Name)
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

// kill 向 dsh-server 所在进程组发信号（负 PID 覆盖组内全部进程）。
func (p *Process) kill(sig syscall.Signal) {
	if p.cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-p.cmd.Process.Pid, sig)
}

// Stop 终止后端并等待其退出：SIGTERM 整个进程组，stopGrace 内未退出则
// SIGKILL 兜底，随后等待 Wait 收口（exitCh 必然收到结果）。
func (p *Process) Stop() {
	if p.cmd.Process == nil {
		return
	}
	p.kill(syscall.SIGTERM)
	select {
	case <-p.exitCh:
		return
	case <-time.After(stopGrace):
	}
	p.kill(syscall.SIGKILL)
	select {
	case <-p.exitCh:
	case <-time.After(stopGrace):
	}
}

// Start 启动一次 SEA 后端（dsh-server web --port <port>），等待其把
// `dsh web: http://127.0.0.1:<port>` 打到 stdout，返回监听 URL。
// port 传 "0" 时由 OS 分配（默认，避免与已占用端口冲突）。ctx 取消或超时
// 时终止后端并返回错误。成功返回的 Process 由调用方在退出/重启时终止
// 进程组。
func Start(ctx context.Context, exeDir, port string) (*Process, string, error) {
	cmd := command(exeDir, port)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", fmt.Errorf("stdout pipe: %w", err)
	}
	// 后端 stderr 直接透传到壳的 stderr，便于诊断。
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("start %s: %w", Name, err)
	}

	// 进程终结信号：Wait 必须在读取完管道后调用，这里独立收口。
	exitCh := make(chan Exit, 1)
	go func() {
		err := cmd.Wait()
		exitCh <- Exit{Err: err}
	}()
	p := &Process{cmd: cmd, exitCh: exitCh}

	// 逐行读 stdout，直到出现 URL 行；之后继续读到 EOF，防止后端持续写
	// stdout（agent 运行日志等）时管道缓冲填满而阻塞在 write —— 阻塞会拖住
	// 后端的优雅退出（SIGTERM handler 里 dispose 也写 stdout），进而拖住
	// Process.Stop 的收口。排空到 EOF 也满足 exec.Cmd.Wait 要求先读完管道。
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
			if !urlSent && strings.HasPrefix(line, readyPrefix) {
				urlCh <- strings.TrimSpace(strings.TrimPrefix(line, readyPrefix))
				urlSent = true
			}
		}
	}()

	select {
	case u := <-urlCh:
		if u == "" {
			p.Stop()
			return p, "", fmt.Errorf("server exited without publishing a URL")
		}
		return p, u, nil
	case <-time.After(urlTimeout):
		p.Stop()
		return p, "", fmt.Errorf("timed out waiting for dsh web URL")
	case <-ctx.Done():
		p.Stop()
		return p, "", ctx.Err()
	}
}
