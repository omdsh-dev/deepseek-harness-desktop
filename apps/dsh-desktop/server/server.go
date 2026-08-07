// Package server 管理 SEA 后端（dsh-server）进程的生命周期：构造启动命令、
// 等待就绪 URL、终止进程。不依赖 Wails，可独立测试。
//
// dsh-server 是桌面应用壳同目录下的 SEA 可执行（内嵌 node 的 `dsh web`），
// 启动参数 `web --port <port>`（端口传 "0" 由 OS 分配，避免冲突）。就绪后
// 它把 `dsh web: http://127.0.0.1:<port>` 打到 stdout，本包解析该行得到
// 实际监听地址。终止语义平台相关：Unix 按独立进程组发信号（Setpgid），
// Windows 用 Job Object 按作业树终止（CREATE_NEW_PROCESS_GROUP），实现见
// server_unix.go / server_windows.go。
package server

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// readyPrefix 是后端就绪行前缀；壳从此解析实际监听 URL。
const readyPrefix = "dsh web: "

// 后端 URL 就绪的等待上限，以及优雅终止后等待退出的宽限期
// （到期未退出则强杀兜底）。
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
	cmd     *exec.Cmd
	exitCh  <-chan Exit
	cleanup func()  // 平台注册的资源清理（Windows: Job Object 句柄；Unix: nil）
	job     uintptr // 平台句柄（Windows: Job Object；Unix: 恒 0）
}

// Exit 返回进程终结结果；进程退出后收到。
func (p *Process) Exit() <-chan Exit {
	return p.exitCh
}

// done 释放平台资源（幂等；进程终结后调用）。
func (p *Process) done() {
	if p.cleanup != nil {
		p.cleanup()
		p.cleanup = nil
	}
}

// Stop 终止后端并等待其退出：先优雅终止（Unix SIGTERM 进程组 / Windows
// TerminateJobObject），stopGrace 内未退出则强杀兜底（Unix SIGKILL /
// Windows TerminateJobObject 幂等），随后等待 Wait 收口（exitCh 必然收到
// 结果）并释放平台资源。
func (p *Process) Stop() {
	if p.cmd.Process == nil {
		return
	}
	requestStop(p)
	select {
	case <-p.exitCh:
		p.done()
		return
	case <-time.After(stopGrace):
	}
	forceStop(p)
	select {
	case <-p.exitCh:
	case <-time.After(stopGrace):
	}
	p.done()
}

// Start 启动一次 SEA 后端（dsh-server web --port <port>），等待其把
// `dsh web: http://127.0.0.1:<port>` 打到 stdout，返回监听 URL。
// port 传 "0" 时由 OS 分配（默认，避免与已占用端口冲突）。ctx 取消或超时
// 时终止后端并返回错误。成功返回的 Process 由调用方在退出/重启时终止。
func Start(ctx context.Context, exeDir, port string) (*Process, string, error) {
	cmd := command(exeDir, port)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", fmt.Errorf("stdout pipe: %w", err)
	}
	// 后端 stderr 直接透传到壳的 stderr，便于诊断。
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("start dsh-server: %w", err)
	}

	// 进程终结信号：Wait 必须在读取完管道后调用，这里独立收口。
	exitCh := make(chan Exit, 1)
	go func() {
		err := cmd.Wait()
		exitCh <- Exit{Err: err}
	}()
	p := &Process{cmd: cmd, exitCh: exitCh}
	if err := attachToJob(p); err != nil {
		p.Stop()
		return p, "", fmt.Errorf("attach dsh-server to job: %w", err)
	}

	// 逐行读 stdout，直到出现 URL 行；之后继续读到 EOF，防止后端持续写
	// stdout（agent 运行日志等）时管道缓冲填满而阻塞在 write —— 阻塞会拖住
	// 后端的退出，进而拖住 Process.Stop 的收口。排空到 EOF 也满足
	// exec.Cmd.Wait 要求先读完管道。
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
