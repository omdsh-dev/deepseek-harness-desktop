package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/omdsh-dev/dsh-web-desktopify/internal/config"
)

// TestShellBuildE2E 验证 buildShell 能解出内嵌源码、动态生成 go.mod 并
// go build 出壳二进制。
func TestShellBuildE2E(t *testing.T) {
	ws := t.TempDir()
	cfg := &config.Config{Name: "e2e-shell"}
	out, err := buildShell(ws, cfg)
	if err != nil {
		t.Fatalf("buildShell: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("壳二进制不存在: %v", err)
	}
	srcDir := filepath.Join(config.BuildDir(ws, cfg), ".shell-src")
	gomod, err := os.ReadFile(filepath.Join(srcDir, "go.mod"))
	if err != nil {
		t.Fatalf("读 go.mod: %v", err)
	}
	if !strings.HasPrefix(string(gomod), "module github.com/omdsh-dev/dsh-web-desktopify/pkg/shell") {
		t.Fatalf("go.mod 模块名错误: %s", gomod)
	}
	if !strings.Contains(string(gomod), "wailsapp/wails/v3") {
		t.Fatalf("go.mod 应含 wails 依赖")
	}
	if !dirExists(filepath.Join(srcDir, "cmd")) {
		t.Fatal("解出的源码应有 cmd/ 目录")
	}
}
