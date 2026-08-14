package fsutil

import "testing"

// TestNativeSkip 验证平台原生二进制过滤：只保留当前平台的 prebuilds
// 目录与平台变体包，且不误伤普通包与散文件。
func TestNativeSkip(t *testing.T) {
	cur := nativePlatform
	other := "win32-x64"
	if cur == other {
		other = "linux-x64"
	}
	cases := []struct {
		rel   string
		isDir bool
		want  bool
	}{
		// prebuilds 目录：当前平台保留，其他平台跳过。
		{"node-pty/prebuilds/" + cur, true, false},
		{"node-pty/prebuilds/" + other, true, true},
		// prebuilds 下散文件不误伤。
		{"node-pty/prebuilds/foo.bin", false, false},
		// 平台变体包：当前平台保留，其他平台跳过（含编译器后缀）。
		{"@img/sharp-" + cur, true, false},
		{"@img/sharp-" + other, true, true},
		{"@img/sharp-linux-x64-gnu", true, cur != "linux-x64"},
		{"@koromix/koffi-" + cur, true, false},
		{"@koromix/koffi-" + other, true, true},
		{"node-addon-require-builtin-" + cur, true, false},
		// 家族前缀巧合但非平台变体：不误伤。
		{"sharp-helpers", true, false},
		{"@img/sharp-lib", true, false},
		// 普通包。
		{"commander", true, false},
		{"@deepseek-ai/dsh", true, false},
	}
	for _, c := range cases {
		if got := NativeSkip(c.rel, c.isDir); got != c.want {
			t.Errorf("NativeSkip(%q, %v) = %v, want %v", c.rel, c.isDir, got, c.want)
		}
	}
}
