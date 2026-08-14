package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWithEnv(t *testing.T) {
	env := []string{"HOME=/home/u", "DSH_HOME=/old", "PATH=/usr/bin"}
	got := withEnv(env, "DSH_HOME", "/new")
	if len(got) != 3 {
		t.Fatalf("长度应为 3，得到 %v", got)
	}
	if got[0] != "HOME=/home/u" || got[1] != "PATH=/usr/bin" || got[2] != "DSH_HOME=/new" {
		t.Fatalf("DSH_HOME 应被替换并追加在末尾，得到 %v", got)
	}
	// 不存在时追加。
	got = withEnv(env, "NEW_KEY", "v")
	if len(got) != 4 || got[3] != "NEW_KEY=v" {
		t.Fatalf("应追加新键，得到 %v", got)
	}
}

func TestPrependPath(t *testing.T) {
	env := []string{"HOME=/home/u", "PATH=/usr/bin:/bin", "DSH_HOME=/old"}
	got := prependPath(env, "/opt/pnpm")
	var path string
	for _, e := range got {
		if strings.HasPrefix(e, "PATH=") {
			path = e
		}
	}
	if path != "PATH=/opt/pnpm"+string(os.PathListSeparator)+"/usr/bin:/bin" {
		t.Fatalf("PATH 应前置 /opt/pnpm 且只保留一个条目，得到 %q", path)
	}
	if len(got) != 3 {
		t.Fatalf("其他条目应保留，得到 %v", got)
	}
}

func TestEnsureProfileLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 符号链接需要特权")
	}
	home := t.TempDir()
	ws := filepath.Join(home, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, "profiles", "web")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}

	// 不存在时创建。
	if err := ensureProfileLink(link, ws); err != nil {
		t.Fatalf("创建链接: %v", err)
	}
	want, err := filepath.EvalSymlinks(ws)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := filepath.EvalSymlinks(link); err != nil || got != want {
		t.Fatalf("链接应指向 %s，得到 %s（%v）", want, got, err)
	}

	// 已指向同一工作区：幂等通过。
	if err := ensureProfileLink(link, ws); err != nil {
		t.Fatalf("同目标应通过: %v", err)
	}

	// 指向别处：拒绝。
	other := filepath.Join(home, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, link); err != nil {
		t.Skipf("创建符号链接失败: %v", err)
	}
	if err := ensureProfileLink(link, ws); err == nil {
		t.Fatal("指向别处的链接应报错")
	}

	// 普通目录（非符号链接）：拒绝。
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(link, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureProfileLink(link, ws); err == nil {
		t.Fatal("普通目录应报错")
	}
}
