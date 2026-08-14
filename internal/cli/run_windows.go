//go:build windows

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// runDetachedEnv 以额外环境变量启动进程并脱离当前终端（CLI 立即返回）。
func runDetachedEnv(bin string, extraEnv []string) error {
	cmd := exec.Command(bin)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008} // DETACHED_PROCESS
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 %s: %w", bin, err)
	}
	// 释放句柄，让子进程独立于 CLI 存活。
	go cmd.Wait()
	return nil
}
