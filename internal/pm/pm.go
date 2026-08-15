// Package pm 提供包管理器 pnpm 的命令构造。
//
// 直接使用 PATH 上的 pnpm，不假设 mise/nub 等特定版本管理器：pnpm
// 的配置（.npmrc、pnpm-workspace.yaml 的 allowBuilds 等）由 pnpm 自身
// 解析，与调用方无关；版本选择由 PATH 上的 pnpm（或它的 shim）负责。
package pm

import (
	"fmt"
	"os/exec"
)

// Command 返回一个执行 pnpm 的命令（args 为 pnpm 参数）。
func Command(args ...string) (*exec.Cmd, error) {
	bin, err := Bin()
	if err != nil {
		return nil, err
	}
	return exec.Command(bin, args...), nil
}

// Bin 返回 pnpm 可执行文件路径：PATH 查找（版本管理器 shim 也可用）。
func Bin() (string, error) {
	p, err := exec.LookPath("pnpm")
	if err != nil {
		return "", fmt.Errorf("未找到 pnpm：请安装 pnpm 并确保在 PATH（如 npm i -g pnpm 或 corepack enable）")
	}
	return p, nil
}
