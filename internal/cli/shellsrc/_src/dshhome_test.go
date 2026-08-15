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
// （与 dev 的运行时 home 一致，不再加 dsh-home 子目录）；home 根的用户
// 数据（种子里没有）不受 profile 落位影响。
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
	if err := os.WriteFile(filepath.Join(profileDir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, ".seed-hash"), []byte("h1"), 0o644); err != nil {
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
	if hash := readSeedHash(filepath.Join(got, "profiles", "web", ".seed-hash")); hash != "h1" {
		t.Fatalf(".seed-hash 应复制为 h1，得到 %q", hash)
	}

	// 用户数据在 home 根（种子里没有）：不被 profile 落位删除。
	userData := filepath.Join(got, "user-data.txt")
	if err := os.WriteFile(userData, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 二次启动：指纹一致，跳过覆盖。
	if _, err := resolveDSHHome(cfg, filepath.Join(seedRoot, "bin")); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(userData); err != nil || string(raw) != "keep" {
		t.Fatalf("用户数据不应被影响（%q, %v）", raw, err)
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

// TestEnsureSeedOverridesStaleEntity：指纹不一致的旧实体拷贝被种子强制
// 覆盖（profile 定义随应用更新）。
func TestEnsureSeedOverridesStaleEntity(t *testing.T) {
	seedRoot := t.TempDir()
	seed := filepath.Join(seedRoot, "dsh-home")
	seedProfile := filepath.Join(seed, "profiles", "web")
	if err := os.MkdirAll(seedProfile, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedProfile, "package.json"), []byte(`{"v":"new"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedProfile, ".seed-hash"), []byte("hash-new"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "home")
	dstProfile := filepath.Join(dst, "profiles", "web")
	if err := os.MkdirAll(dstProfile, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dstProfile, "package.json"), []byte(`{"v":"old"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dstProfile, ".seed-hash"), []byte("hash-old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensureSeed(seed, dst, "web"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dstProfile, "package.json"))
	if err != nil || string(raw) != `{"v":"new"}` {
		t.Fatalf("旧实体应被种子覆盖（%q, %v）", raw, err)
	}
	hash, err := os.ReadFile(filepath.Join(dstProfile, ".seed-hash"))
	if err != nil || string(hash) != "hash-new" {
		t.Fatalf(".seed-hash 应更新为 hash-new，得到 %q（%v）", hash, err)
	}
}

// TestEnsureSeedSkipsMatchingFingerprint：指纹一致时跳过覆盖（同一版本
// 正常启动不重复复制闭包）。
func TestEnsureSeedSkipsMatchingFingerprint(t *testing.T) {
	seedRoot := t.TempDir()
	seed := filepath.Join(seedRoot, "dsh-home")
	seedProfile := filepath.Join(seed, "profiles", "web")
	if err := os.MkdirAll(seedProfile, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedProfile, "package.json"), []byte(`{"v":"seed"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedProfile, ".seed-hash"), []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "home")
	dstProfile := filepath.Join(dst, "profiles", "web")
	if err := os.MkdirAll(dstProfile, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dstProfile, "package.json"), []byte(`{"v":"user"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dstProfile, ".seed-hash"), []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensureSeed(seed, dst, "web"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dstProfile, "package.json"))
	if err != nil || string(raw) != `{"v":"user"}` {
		t.Fatalf("指纹一致应跳过覆盖（%q, %v）", raw, err)
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
