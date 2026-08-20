package dshhome

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"

	"github.com/omdsh-dev/dsh-web-desktopify/pkg/shell/appconfig"
)

func TestResolveEnv(t *testing.T) {
	cfg := appconfig.Default()
	cfg.DSHHome = "env"
	got, err := Resolve(cfg, "/tmp/exe")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("env 策略应返回空串，得到 %q", got)
	}
}

func TestResolveOverride(t *testing.T) {
	t.Setenv("DSH_APP_DSH_HOME", "/override/home")
	cfg := appconfig.Default()
	cfg.DSHHome = "xdg"
	got, err := Resolve(cfg, "/tmp/exe")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/override/home" {
		t.Fatalf("应返回环境变量覆盖值，得到 %q", got)
	}
}

// xdg 策略把种子拷贝到 XDG_DATA_HOME/<name>；home 根的用户数据不受影响。
func TestResolveXdg(t *testing.T) {
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

	cfg := appconfig.Default()
	cfg.Name = "dsh-test"
	cfg.DSHHome = "xdg"
	cfg.Profile = "web"
	got, err := Resolve(cfg, filepath.Join(seedRoot, "bin"))
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
	if _, err := Resolve(cfg, filepath.Join(seedRoot, "bin")); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(userData); err != nil || string(raw) != "keep" {
		t.Fatalf("用户数据不应被影响（%q, %v）", raw, err)
	}
}

func TestResolveFixedPath(t *testing.T) {
	seedRoot := t.TempDir()
	seed := filepath.Join(seedRoot, "dsh-home")
	if err := os.MkdirAll(filepath.Join(seed, "profiles", "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "profiles", "web", "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "home")
	cfg := appconfig.Default()
	cfg.Profile = "web"
	cfg.DSHHome = dst
	got, err := Resolve(cfg, filepath.Join(seedRoot, "bin"))
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

func TestResolveBadPath(t *testing.T) {
	cfg := appconfig.Default()
	cfg.DSHHome = "relative/path"
	if _, err := Resolve(cfg, "/tmp/exe"); err == nil {
		t.Fatal("相对路径应报错")
	}
}

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

// dev 模式留下的 profiles/web 符号链接不是有效种子——app 启动应移除它并
// 复制实体种子，使 app 独立于工作区。
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

	cfg := appconfig.Default()
	cfg.Profile = "web"
	cfg.DSHHome = dst
	got, err := Resolve(cfg, filepath.Join(seedRoot, "bin"))
	if err != nil {
		t.Fatal(err)
	}
	if got != dst {
		t.Fatalf("应返回固定路径 %q，得到 %q", dst, got)
	}
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
