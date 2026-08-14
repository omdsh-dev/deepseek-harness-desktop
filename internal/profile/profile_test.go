package profile

import (
	"os"
	"path/filepath"
	"testing"
)

// makePkg 在 nmDir 下创建包目录（支持 @scope/name）与 package.json。
func makePkg(t *testing.T, nmDir, name, version string) {
	t.Helper()
	dir := filepath.Join(nmDir, filepath.FromSlash(name))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"name":"` + name + `","version":"` + version + `"}`)
	if err := os.WriteFile(filepath.Join(dir, "package.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClosureFingerprint(t *testing.T) {
	dir := t.TempDir()
	nmDir := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(nmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 顶层包与 scoped 包、簿记条目混合。
	makePkg(t, nmDir, "commander", "15.0.0")
	makePkg(t, nmDir, "@deepseek-ai/dsh", "0.1.0-rc.6")
	makePkg(t, nmDir, "@deepseek-ai/dsh-base", "0.1.0-rc.6")
	if err := os.MkdirAll(filepath.Join(nmDir, ".bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nmDir, ".modules.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	f1, err := ClosureFingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if len(f1) != 64 {
		t.Fatalf("指纹长度异常: %q", f1)
	}

	// 稳定：相同闭包再次计算一致。
	f2, err := ClosureFingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if f1 != f2 {
		t.Fatalf("相同闭包指纹不稳定: %s vs %s", f1, f2)
	}

	// 敏感：新增包使指纹变化（模拟 pnpm install 补齐闭包）。
	makePkg(t, nmDir, "ms", "2.1.3")
	f3, err := ClosureFingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if f1 == f3 {
		t.Fatalf("闭包变化但指纹未变: %s", f1)
	}

	// 敏感：升级版本使指纹变化。
	makePkg(t, nmDir, "commander", "16.0.0")
	f4, err := ClosureFingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if f3 == f4 {
		t.Fatalf("版本升级但指纹未变: %s", f3)
	}

	// node_modules 缺失：空指纹不报错。
	empty := filepath.Join(t.TempDir(), "no-nm")
	f5, err := ClosureFingerprint(empty)
	if err != nil || f5 != "" {
		t.Fatalf("缺失闭包应返回空指纹，得到 %q, %v", f5, err)
	}
}
