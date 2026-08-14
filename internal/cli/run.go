package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/omdsh-dev/deepseek-harness-desktop/internal/config"
)

// buildShell 用仓库根的 Go 模块构建壳二进制（./internal/shell），输出到
// target/<name>/.shell/。
func buildShell(root, ws string, cfg *config.Config) (string, error) {
	outDir := filepath.Join(config.BuildDir(root, cfg), ".shell")
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
