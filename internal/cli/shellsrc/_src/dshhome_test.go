package main

import (
	"github.com/adrg/xdg"
	"os"
	"path/filepath"
	"testing"
)

// TestResolveDSHHomeEnv：env 策略不设置 DSH_HOME。
func TestResolveDSHHomeEnv(t *testing.T) {
	cfg := defaultAppConfig()
	cfg.DSHHome = "env"
	got, err := resolveDSHHome(cfg, "/tmp/exe")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("env 策略应返回空串，得到 %q", got)
	}
}

// TestResolveDSHHomeOverride：DSH_APP_DSH_HOME 优先于一切策略。
func TestResolveDSHHomeOverride(t *testing.T) {
	t.Setenv("DSH_APP_DSH_HOME", "/override/home")
	cfg := defaultAppConfig()
	cfg.DSHHome = "xdg"
	got, err := resolveDSHHome(cfg, "/tmp/exe")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/override/home" {
		t.Fatalf("应返回环境变量覆盖值，得到 %q", got)
	}
}

// TestResolveDSHHomeXdg：xdg 策略把种子内容拷贝到 XDG_DATA_HOME/<name>
// （与 dev 的运行时 home 一致，不再加 dsh-home 子目录）。
func TestResolveDSHHomeXdg(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	xdg.Reload() // xdg 包在 init 时固定路径，env 变更后需刷新
	t.Cleanup(xdg.Reload)

	seedRoot := t.TempDir()
	seed := filepath.Join(seedRoot, "dsh-home")
	profileDir := filepath.Join(seed, "profiles", "web")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "settings.yaml"), []byte("llm: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultAppConfig()
	cfg.Name = "dsh-test"
	cfg.DSHHome = "xdg"
	cfg.Profile = "web"
	got, err := resolveDSHHome(cfg, filepath.Join(seedRoot, "bin"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dataHome, "dsh-test")
	if got != want {
		t.Fatalf("应返回 %q，得到 %q", want, got)
	}
	if !dirExists(filepath.Join(got, "profiles", "web")) {
		t.Fatalf("首次启动应把种子内容拷贝到 %s", got)
	}
	if _, err := os.Stat(filepath.Join(got, "settings.yaml")); err != nil {
		t.Fatalf("种子文件缺失: %v", err)
	}

	// 二次启动：不重新拷贝（数据目录已有 profile）。
	if err := os.RemoveAll(filepath.Join(got, "settings.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveDSHHome(cfg, filepath.Join(seedRoot, "bin")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(got, "settings.yaml")); err == nil {
		t.Fatalf("二次启动不应覆盖已有数据")
	}
}

// TestResolveDSHHomeFixedPath：固定路径策略在 profile 缺失时从种子补齐。
func TestResolveDSHHomeFixedPath(t *testing.T) {
	seedRoot := t.TempDir()
	seed := filepath.Join(seedRoot, "dsh-home")
	if err := os.MkdirAll(filepath.Join(seed, "profiles", "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "profiles", "web", "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "home")
	cfg := defaultAppConfig()
	cfg.Profile = "web"
	cfg.DSHHome = dst
	got, err := resolveDSHHome(cfg, filepath.Join(seedRoot, "bin"))
	if err != nil {
		t.Fatal(err)
	}
	if got != dst {
		t.Fatalf("应返回固定路径 %q，得到 %q", dst, got)
	}
	if !dirExists(filepath.Join(dst, "profiles", "web")) {
		t.Fatalf("固定路径应补齐缺失的 profile")
	}
}

// TestResolveDSHHomeBadPath：相对路径策略报错。
func TestResolveDSHHomeBadPath(t *testing.T) {
	cfg := defaultAppConfig()
	cfg.DSHHome = "relative/path"
	if _, err := resolveDSHHome(cfg, "/tmp/exe"); err == nil {
		t.Fatal("相对路径应报错")
	}
}

// TestEnsureSeedReplacesDevSymlink：dev 模式留下的 profiles/web 符号链接
// （指向工作区）不是有效种子——app 启动应移除它并复制实体种子，使 app
// 独立于工作区（工作区缺失不影响 app）。
func TestEnsureSeedReplacesDevSymlink(t *testing.T) {
	seedRoot := t.TempDir()
	seed := filepath.Join(seedRoot, "dsh-home")
	profileDir := filepath.Join(seed, "profiles", "web")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 模拟 dev 留下的 symlink：profiles/web → 外部工作区。
	dst := filepath.Join(t.TempDir(), "home")
	ws := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(filepath.Join(dst, "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ws, filepath.Join(dst, "profiles", "web")); err != nil {
		t.Fatal(err)
	}

	cfg := defaultAppConfig()
	cfg.Profile = "web"
	cfg.DSHHome = dst
	got, err := resolveDSHHome(cfg, filepath.Join(seedRoot, "bin"))
	if err != nil {
		t.Fatal(err)
	}
	if got != dst {
		t.Fatalf("应返回固定路径 %q，得到 %q", dst, got)
	}
	// symlink 被实体目录替换，且不再指向工作区。
	link := filepath.Join(dst, "profiles", "web")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("profile 应存在: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("dev symlink 应被移除")
	}
	if _, err := os.Stat(filepath.Join(link, "package.json")); err != nil {
		t.Fatalf("应复制实体种子: %v", err)
	}
}
