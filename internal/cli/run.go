package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/omdsh-dev/deepseek-harness-desktop/internal/config"
)

// buildShell 用 desktop 源码树的 Go 模块构建壳二进制（./internal/shell），
// 输出到工作区 target/<name>/.shell/。root 是 desktop 源码 checkout
// （go build 的模块根，须含 go.mod 与 internal/shell），ws 是工作区
// （产物根）。
func buildShell(root, ws string, cfg *config.Config) (string, error) {
	outDir := filepath.Join(config.BuildDir(ws, cfg), ".shell")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	binName := "dsh-shell"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	out := filepath.Join(outDir, binName)
	cmd := exec.Command("go", "build", "-o", out, "./internal/shell")
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go build ./internal/shell: %w", err)
	}
	return out, nil
}
