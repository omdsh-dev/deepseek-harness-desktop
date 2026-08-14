package cli

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/omdsh-dev/deepseek-harness-desktop/internal/cli/shellsrc"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/config"
)

// buildShell 构建壳二进制（Wails v3），输出到工作区
// target/<name>/.shell/。构建输入（精简 go.mod + 壳源码：模块根
// package main + server/ 包）由 shellsrc 内嵌在 CLI 二进制中，运行时
// 解出到 target/<name>/.shell-src/ 再 go build——不依赖外部源码树，
// CLI 可 go install 后脱离仓库运行。
func buildShell(ws string, cfg *config.Config) (string, error) {
	outDir := filepath.Join(config.BuildDir(ws, cfg), ".shell")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	binName := "dsh-shell"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	out := filepath.Join(outDir, binName)
	srcDir, err := materializeShellSrc(ws, cfg)
	if err != nil {
		return "", err
	}
	// 壳是解出模块根的 package main（import .../server），go build . 构建。
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = srcDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// 内嵌输入不含 go.sum（见 shellsrc 包注释）；-mod=mod 让 go 在解出
	// 目录自动补全 go.sum，依赖直接复用 GOMODCACHE（CLI 编译时已下载）。
	cmd.Env = append(os.Environ(), "GOFLAGS="+os.Getenv("GOFLAGS")+" -mod=mod")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go build 壳: %w", err)
	}
	return out, nil
}

// materializeShellSrc 把内嵌的壳构建输入（shellsrc._src，模块根结构：
// go.mod.txt + 壳源码平铺 + server/）解出为临时模块根
// （target/<name>/.shell-src/），返回该根目录。每次全量重写，保证与
// 二进制内嵌内容一致。
func materializeShellSrc(ws string, cfg *config.Config) (string, error) {
	srcDir := filepath.Join(config.BuildDir(ws, cfg), ".shell-src")
	if err := os.RemoveAll(srcDir); err != nil {
		return "", err
	}
	// _src 布局即模块根，整体解出。
	if err := writeEmbedDir(shellsrc.FS, "_src", srcDir); err != nil {
		return "", fmt.Errorf("解出壳源码: %w", err)
	}
	// 还原 go.mod（内嵌时以 .txt 后缀存放，见 shellsrc 包注释）。
	if err := os.Rename(filepath.Join(srcDir, "go.mod.txt"), filepath.Join(srcDir, "go.mod")); err != nil {
		return "", fmt.Errorf("还原 go.mod: %w", err)
	}
	return srcDir, nil
}

// writeEmbedDir 把 fsys 中 dir 前缀下的全部条目写出到 dst 下（去掉前缀）。
func writeEmbedDir(fsys embed.FS, dir, dst string) error {
	return fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fsys.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
