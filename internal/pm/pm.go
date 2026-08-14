// Package pm 提供包管理器 pnpm 的命令构造。
//
// PATH 上的 pnpm 可能是 nub 的 shim（~/.nub/shims/pnpm），它用自己的
// 语义解释配置（不认识 pnpm 的 allowBuilds 等键），会导致安装行为与
// 官方 pnpm 不一致。这里优先用 `mise which pnpm` 解析出 mise 安装的
// 真实 pnpm 二进制，与 dsh 官方（pnpm 生态）保持一致；找不到真实的
// pnpm 时明确报错。
package pm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Command 返回一个执行真实 pnpm 的命令（args 为 pnpm 参数）。
func Command(args ...string) (*exec.Cmd, error) {
	bin, err := Bin()
	if err != nil {
		return nil, err
	}
	return exec.Command(bin, args...), nil
}

// Bin 返回真实 pnpm 可执行文件路径：优先 mise which pnpm，回退 PATH
// （跳过 nub shim）。
func Bin() (string, error) {
	if mise, err := exec.LookPath("mise"); err == nil {
		if out, err := exec.Command(mise, "which", "pnpm").Output(); err == nil {
			p := strings.TrimSpace(string(out))
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				return p, nil
			}
		}
	}
	if p, err := exec.LookPath("pnpm"); err == nil {
		// PATH 命中可能是 nub 的 shim（~/.nub/shims/pnpm）：它不是真实
		// pnpm，配置语义不一致，拒绝使用。
		if strings.Contains(filepath.ToSlash(p), ".nub/shims/") {
			return "", fmt.Errorf("PATH 上的 pnpm 是 nub shim（%s），不是真实 pnpm；请用 mise 安装（mise.toml 声明 pnpm = 'latest'）", p)
		}
		return p, nil
	}
	return "", fmt.Errorf("未找到 pnpm：请用 mise 安装（mise.toml 声明 pnpm = 'latest'），或确保真实 pnpm 在 PATH")
}
