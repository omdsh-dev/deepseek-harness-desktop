package shellsrc

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFSComplete 校验内嵌副本包含构建壳所需的全部文件（模块定义 + 壳
// 源码 + server/），缺失会在编译 CLI 后于运行时暴露。
func TestFSComplete(t *testing.T) {
	// 嵌入内容必须能构成临时模块根：go.mod（.txt）+ 壳入口 + server/。
	for _, want := range []string{
		"_src/go.mod.txt",
		"_src/main.go",
		"_src/appconfig.go",
		"_src/dshhome.go",
		"_src/supervise.go",
		"_src/landing.html",
		"_src/server/server.go",
		"_src/server/server_unix.go",
		"_src/server/server_windows.go",
	} {
		if _, err := fs.Stat(FS, want); err != nil {
			t.Errorf("内嵌副本缺少 %s：%v（先跑 just sync-shell-src）", want, err)
		}
	}
}

// TestFSInSync 校验内嵌副本与仓库真源码一致（源码树内有效），防止修改
// 壳源码后忘记同步。
func TestFSInSync(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// 对照源：壳入口（internal/shell，平铺到 _src）与 server/（模块根）。
	disk := map[string]string{
		"_src/main.go":       filepath.Join(repoRoot, "internal", "shell", "main.go"),
		"_src/appconfig.go":  filepath.Join(repoRoot, "internal", "shell", "appconfig.go"),
		"_src/dshhome.go":    filepath.Join(repoRoot, "internal", "shell", "dshhome.go"),
		"_src/supervise.go":  filepath.Join(repoRoot, "internal", "shell", "supervise.go"),
		"_src/landing.html":  filepath.Join(repoRoot, "internal", "shell", "landing.html"),
		"_src/dshhome_test.go": filepath.Join(repoRoot, "internal", "shell", "dshhome_test.go"),
		"_src/server":        filepath.Join(repoRoot, "server"),
	}
	for embedRel, diskRoot := range disk {
		if err := fs.WalkDir(FS, embedRel, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(embedRel, path)
			if err != nil {
				return err
			}
			if rel == "." {
				return nil
			}
			diskPath := filepath.Join(diskRoot, rel)
			if d.IsDir() {
				info, err := os.Stat(diskPath)
				if err != nil || !info.IsDir() {
					t.Errorf("副本目录 %s 在磁盘上不存在：%v", rel, err)
				}
				return nil
			}
			got, err := FS.ReadFile(path)
			if err != nil {
				return err
			}
			want, err := os.ReadFile(diskPath)
			if err != nil {
				t.Errorf("副本文件 %s 在磁盘上不存在：%v", rel, err)
				return nil
			}
			if string(got) != string(want) {
				t.Errorf("副本 %s 与磁盘不一致（先跑 just sync-shell-src）", rel)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// TestGoModMinimal 校验内嵌 go.mod 是精简版（只含壳依赖，由 sync 脚本
// go mod tidy 生成），防止被意外替换回主 go.mod 全量副本。
func TestGoModMinimal(t *testing.T) {
	got, err := FS.ReadFile("_src/go.mod.txt")
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	// 壳实际依赖必须保留。
	for _, want := range []string{
		"github.com/adrg/xdg",
		"github.com/wailsapp/wails/v3",
		"golang.org/x/sys",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("内嵌 go.mod 缺少壳依赖 %s（先跑 just sync-shell-src）", want)
		}
	}
	// CLI 专用依赖与 tool 指令不应出现。
	for _, banned := range []string{
		"tool github.com/omdsh-dev",
		"github.com/git-pkgs/gitignore",
		"github.com/srwiley/oksvg",
		"github.com/srwiley/rasterx",
		"golang.org/x/image",
	} {
		if strings.Contains(s, banned) {
			t.Errorf("内嵌 go.mod 不应包含 %s（先跑 just sync-shell-src）", banned)
		}
	}
}
