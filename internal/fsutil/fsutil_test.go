package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCopyDirDerefSkipsSelf：复制到源内部的目标时，目标自身的子树必须被
// 跳过（避免递归自复制），不依赖任何目录名约定。
func TestCopyDirDerefSkipsSelf(t *testing.T) {
	src := t.TempDir()
	// 源里放一个产物目录（模拟 bundle 已生成的 target/）。
	artifact := filepath.Join(src, "target", "dsh-coding", "dsh-coding.app")
	if err := os.MkdirAll(artifact, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifact, "dsh-shell"), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 源里一个普通文件。
	if err := os.WriteFile(filepath.Join(src, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 目标在源内部（bundle 的 dsh-home 种子场景）。
	dst := filepath.Join(src, "target", "dsh-coding", "dsh-home", "profiles", "web")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CopyDirDeref(src, dst, nil, nil); err != nil {
		t.Fatal(err)
	}

	// 目标里不应出现嵌套的 target 或 dsh-home（自复制被跳过）。
	bad := filepath.Join(dst, "target")
	if _, err := os.Stat(bad); err == nil {
		t.Fatalf("目标不应包含自身子树 %s（递归自复制）", bad)
	}
	// package.json 应正常复制。
	if _, err := os.Stat(filepath.Join(dst, "package.json")); err != nil {
		t.Fatalf("package.json 应被复制: %v", err)
	}
}

// TestCopyDirDerefIgnores：ignored 回调（如 .gitignore）生效。
func TestCopyDirDerefIgnores(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "keep.txt"), []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "drop.txt"), []byte("d"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "ignored-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "ignored-dir", "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ignored := func(rel string, isDir bool) bool {
		return rel == "drop.txt" || rel == "ignored-dir"
	}
	dst := t.TempDir()
	if err := CopyDirDeref(src, dst, nil, ignored); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "keep.txt")); err != nil {
		t.Error("keep.txt 应被复制")
	}
	if _, err := os.Stat(filepath.Join(dst, "drop.txt")); err == nil {
		t.Error("drop.txt 不应被复制（ignored）")
	}
	if _, err := os.Stat(filepath.Join(dst, "ignored-dir")); err == nil {
		t.Error("ignored-dir 不应被复制（ignored）")
	}
}
