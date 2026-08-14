package sea

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckBareImports(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sea-entry.mjs")

	// 全 node: 导入：通过。
	good := `import { createRequire } from "node:module";
import { Command } from "node:os";
const x = await import("node:fs");
`
	if err := os.WriteFile(path, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkBareImports(path); err != nil {
		t.Fatalf("全 node: 导入不应报错: %v", err)
	}

	// 含裸导入（闭包缺包时 bundler 保留）：报错并列出。
	bad := `import { Command, CommanderError } from "commander";
import "side-effect-pkg";
import { readFile } from "node:fs";
`
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	err := checkBareImports(path)
	if err == nil {
		t.Fatal("裸导入应报错")
	}
	for _, want := range []string{"commander", "side-effect-pkg"} {
		if !contains(err.Error(), want) {
			t.Errorf("报错应提及 %s，得到: %v", want, err)
		}
	}

	// 动态 import() 的裸 specifier 报错；CJS require() 走 createRequire
	// 外部解析（原生模块外置），不视为缺陷。
	dyn := `const m = await import("dynamic-pkg");
const c = require("cjs-pkg");
const ok = await import("node:fs");
`
	if err := os.WriteFile(path, []byte(dyn), 0o644); err != nil {
		t.Fatal(err)
	}
	err = checkBareImports(path)
	if err == nil {
		t.Fatal("动态 import 裸导入应报错")
	}
	if !contains(err.Error(), "dynamic-pkg") {
		t.Errorf("报错应提及 dynamic-pkg，得到: %v", err)
	}
	if contains(err.Error(), "cjs-pkg") {
		t.Errorf("require() 不应报错，得到: %v", err)
	}

	// 重复裸导入去重。
	dup := `import { a } from "foo";
export { b } from "foo";
`
	if err := os.WriteFile(path, []byte(dup), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkBareImports(path); err == nil {
		t.Fatal("裸导入应报错")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
