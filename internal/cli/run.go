package cli

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/omdsh-dev/dsh-web-desktopify/internal/config"
	"github.com/omdsh-dev/dsh-web-desktopify/pkg/shell"
)

// buildShell 构建壳二进制（Wails v3）到 target/<name>/.shell/。构建输入由
// shell 包内嵌在 CLI 二进制中，运行时解出并动态生成 go.mod 后 go build——
// 不依赖外部源码树，CLI 可 go install 后脱离仓库运行。
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
	cmd := exec.Command("go", "build", "-o", out, "./cmd")
	cmd.Dir = srcDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// -mod=mod 自动补全 go.sum（go.mod 动态生成）；GOWORK=off 让壳模块
	// 脱离仓库 go.work 单独解析。
	cmd.Env = setEnv(os.Environ(),
		"GOFLAGS", os.Getenv("GOFLAGS")+" -mod=mod",
		"GOWORK", "off",
	)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go build 壳: %w", err)
	}
	return out, nil
}

// setEnv 返回在 env 基础上覆盖指定 key/value 的新环境切片（去掉重复 key）。
func setEnv(env []string, kvs ...string) []string {
	out := make([]string, 0, len(env)+len(kvs))
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			dup := false
			for k := 0; k < len(kvs); k += 2 {
				if kv[:i] == kvs[k] {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
		}
		out = append(out, kv)
	}
	for k := 0; k < len(kvs); k += 2 {
		out = append(out, kvs[k]+"="+kvs[k+1])
	}
	return out
}

// materializeShellSrc 把内嵌的壳构建输入（shell.FS）解出为临时模块根
// （target/<name>/.shell-src/）并动态写入 go.mod，返回该根目录。每次全量
// 重写，保证与二进制内嵌内容一致。
func materializeShellSrc(ws string, cfg *config.Config) (string, error) {
	srcDir := filepath.Join(config.BuildDir(ws, cfg), ".shell-src")
	if err := os.RemoveAll(srcDir); err != nil {
		return "", err
	}
	// shell.FS 根即模块内容，整体解出。
	if err := writeEmbedDir(shell.FS, ".", srcDir); err != nil {
		return "", fmt.Errorf("解出壳源码: %w", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "go.mod"), []byte(shellGoMod()), 0o644); err != nil {
		return "", fmt.Errorf("写壳 go.mod: %w", err)
	}
	return srcDir, nil
}

// shellGoMod 返回壳模块的 go.mod 内容。module 名设为壳源码根包
// github.com/omdsh-dev/dsh-web-desktopify/pkg/shell，使 pkg/shell/... 的
// import 在解出的壳模块中解析为本地子目录。go 版本行跟随当前工具链。
func shellGoMod() string {
	return "module github.com/omdsh-dev/dsh-web-desktopify/pkg/shell\n" +
		"\n" +
		"go " + strings.TrimPrefix(runtime.Version(), "go") + "\n" +
		"\n" +
		"require (\n" +
		"\tgithub.com/adrg/xdg v0.5.3\n" +
		"\tgithub.com/wailsapp/wails/v3 v3.0.0-beta.8\n" +
		"\tgolang.org/x/sys v0.47.0\n" +
		")\n"
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
