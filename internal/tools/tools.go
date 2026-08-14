// Package tools 管理构建工具链（target/tools/）。
//
// 根仓库是纯 Go，不提交任何 npm 清单；构建工具（tsdown 打包 SEA、sharp
// 渲染图标）按需安装到 target/tools（nub install，构建目录本地 store），
// 与工作区解耦。target/ 统一承载全部产物，无其他临时目录。
//
// 工具链的工程文件模板（package.json / .npmrc / pnpm-workspace.yaml）以
// go:embed 内嵌在 templates/ 下。
package tools

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed all:templates
var templates embed.FS

// DirName 是工具链目录名（target/ 下）。
const DirName = "tools"

// Dir 返回工具链目录。
func Dir(root string) string {
	return filepath.Join(root, "target", DirName)
}

// Ensure 确保工具链已安装，返回工具链目录。已安装（tsdown 可执行存在）
// 时跳过安装。
func Ensure(root string) (string, error) {
	dir := Dir(root)
	bin := filepath.Join(dir, "node_modules", ".bin", "tsdown")
	if _, err := os.Stat(bin); err == nil {
		return dir, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir tools %s: %w", dir, err)
	}
	entries, err := templates.ReadDir("templates")
	if err != nil {
		return "", fmt.Errorf("read embedded templates: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := templates.ReadFile("templates/" + e.Name())
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", e.Name(), err)
		}
	}
	cmd := exec.Command("nub", "install")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("nub install（%s）: %w", dir, err)
	}
	return dir, nil
}

// Run 运行工具链里已安装的 bin（如 tsdown、node 脚本），cwd 为 dir。
func Run(root, dir, bin string, args ...string) error {
	path := filepath.Join(Dir(root), "node_modules", ".bin", bin)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("tools bin %s 缺失（先 Ensure）: %w", bin, err)
	}
	cmd := exec.Command(path, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", bin, err)
	}
	return nil
}

// Node 返回 PATH 中的 node（mise 提供），供运行工具链里的 .mjs 脚本。
func Node() (string, error) {
	path, err := exec.LookPath("node")
	if err != nil {
		return "", fmt.Errorf("node 不在 PATH（mise 提供）: %w", err)
	}
	return path, nil
}
