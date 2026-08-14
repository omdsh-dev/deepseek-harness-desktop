package gitignore

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadMissing：无 .gitignore 时不忽略任何内容。
func TestLoadMissing(t *testing.T) {
	m, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if m.Ignored("anything", false) {
		t.Fatal("无 .gitignore 时不应忽略任何内容")
	}
}

// TestBasenameAnyLevel：无斜杠模式匹配任意层级。
func TestBasenameAnyLevel(t *testing.T) {
	m := mustLoad(t, "target/\n*.log\n")
	cases := []struct {
		rel   string
		isDir bool
		want  bool
	}{
		{"target", true, true},
		{"target/dsh-coding/app", true, true},
		{"a/b/target", true, true},
		{"deepseek.log", false, true},
		{"a/b/deepseek.log", false, true},
		{"deepseek.logs", false, false},
		{"src/main.go", false, false},
	}
	for _, c := range cases {
		if got := m.Ignored(c.rel, c.isDir); got != c.want {
			t.Errorf("Ignored(%q, dir=%v) = %v, want %v", c.rel, c.isDir, got, c.want)
		}
	}
}

// TestAnchored：含斜杠模式相对仓库根锚定。
func TestAnchored(t *testing.T) {
	m := mustLoad(t, "/build/\nfoo/bar\n")
	if !m.Ignored("build", true) {
		t.Error("根 build 目录应被忽略")
	}
	if m.Ignored("a/build", true) {
		t.Error("非根的 build 不应被忽略（锚定）")
	}
	if !m.Ignored("foo/bar", false) {
		t.Error("foo/bar 应被忽略")
	}
	if m.Ignored("x/foo/bar", false) {
		t.Error("非根的 foo/bar 不应被忽略（锚定）")
	}
}

// TestNegation：! 取反生效（last match wins）。
func TestNegation(t *testing.T) {
	m := mustLoad(t, "*.log\n!keep.log\n")
	if !m.Ignored("a.log", false) {
		t.Error("a.log 应被忽略")
	}
	if m.Ignored("keep.log", false) {
		t.Error("keep.log 被 ! 取反后不应被忽略")
	}
}

// TestDirOnly：尾斜杠模式仅匹配目录。
func TestDirOnly(t *testing.T) {
	m := mustLoad(t, "cache/\n")
	if !m.Ignored("cache", true) {
		t.Error("cache 目录应被忽略")
	}
	if m.Ignored("cache", false) {
		t.Error("同名文件不应被目录限定 pattern 忽略")
	}
}

// TestGlobStar：** 通配。
func TestGlobStar(t *testing.T) {
	m := mustLoad(t, "**/generated/\na/**/b\n")
	if !m.Ignored("x/y/generated", true) {
		t.Error("**/generated 应匹配任意层级")
	}
	if !m.Ignored("generated", true) {
		t.Error("**/generated 应匹配根层级")
	}
	if !m.Ignored("a/b", false) {
		t.Error("a/**/b 应匹配 a/b")
	}
	if !m.Ignored("a/x/y/b", false) {
		t.Error("a/**/b 应匹配中间多层")
	}
}

// TestCharacterClass：字符类。
func TestCharacterClass(t *testing.T) {
	m := mustLoad(t, "file[0-9].txt\n")
	if !m.Ignored("file3.txt", false) {
		t.Error("file3.txt 应匹配字符类")
	}
	if m.Ignored("filex.txt", false) {
		t.Error("filex.txt 不应匹配 [0-9]")
	}
}

// TestNestedGitignore：子目录 .gitignore 限定其作用域。
func TestNestedGitignore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("top.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, ".gitignore"), []byte("sub.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Ignored("top.log", false) {
		t.Error("顶层规则应全局生效")
	}
	if !m.Ignored("sub/top.log", false) {
		t.Error("无斜杠模式应匹配任意层级（含子目录）")
	}
	if !m.Ignored("sub/sub.log", false) {
		t.Error("子目录规则应作用于其下")
	}
	if m.Ignored("other/sub.log", false) {
		t.Error("子目录规则不应作用于其他目录")
	}
}

func mustLoad(t *testing.T, content string) *Matcher {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
