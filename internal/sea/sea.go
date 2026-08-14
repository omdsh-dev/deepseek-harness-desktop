// Package sea 把已安装的 dsh profile 打包为 SEA（Single Executable
// Application）单文件后端。
//
// 打包完全在 target/<name>/sea/ 内完成：
//  1. 闭包：构建出的 profile 的 node_modules（扁平布局）解引用 symlink
//     复制为 sea/node_modules（SEA 运行时全部依赖的解析根，含 cordis
//     插件闭包）；
//  2. dsh-bridge：向闭包写入 CJS 桥（node_modules/dsh-bridge），运行时
//     经 createRequire 从可执行文件旁的闭包加载，其 import() 走正常
//     Node ESM loader 加载 dsh CLI —— 原生模块、顶层 await 与运行时
//     资源随闭包外置，不依赖 bundler 内联；
//  3. 资源：@deepseek-ai/dsh 包的 config/ 与 package.json 复制为
//     sea/config 与 sea/package.json（运行时 dsh 从闭包内解析自身
//     config；此处副本保留兼容）；
//  4. 生成 sea-entry.mjs（薄入口：仅 builtin 导入 + require dsh-bridge）
//     与 tsdown.config.mjs（不内联任何依赖，node --build-sea）；
//  5. 调用工具链 tsdown 产出 sea/bin/dsh。
package sea

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/omdsh-dev/deepseek-harness-desktop/internal/config"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/fsutil"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/profile"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/tools"
)

//go:embed all:templates
var templates embed.FS

// 复制时排除的 node_modules 簿记条目（非包实体）。
var skipEntries = map[string]bool{
	".store":        true,
	".nub":          true,
	".modules.yaml": true,
}

// 打包模板（sea-entry.mjs / tsdown.config.mjs）以 go:embed 内嵌在
// templates/ 下。entry 是薄入口：只含 node: builtin 导入与
// require("dsh-bridge")；tsdown 不内联任何依赖，用 Node 的 --build-sea
// 生成单文件可执行，dsh CLI、依赖闭包、原生模块与运行时资源全部外置
// 在可执行文件旁（闭包即本目录 node_modules），运行期走正常 Node 解析。

// bridgeName 是闭包内 CJS 桥的包名（sea.Build 写入 node_modules 下）。
const bridgeName = "dsh-bridge"

// bridgePkgJSON / bridgeIndex 是 dsh-bridge 桥的内容：CJS 模块经
// createRequire 从文件系统加载后，其动态 import() 走正常 Node ESM
// loader（blob 内模块的 import() 受 SEA 限制），加载 dsh CLI 及其含
// 顶层 await 的依赖图。
const bridgePkgJSON = `{
  "name": "dsh-bridge",
  "version": "0.0.0",
  "type": "commonjs",
  "main": "index.cjs"
}
`

const bridgeIndex = `// dsh SEA 外部桥（deepseek-harness-desktop 生成）：经 createRequire
// 从可执行文件旁 node_modules 加载的 CJS 模块，其 import() 走正常 Node
// ESM loader，加载 dsh CLI（lib/bin.js）及含顶层 await 的依赖图。
'use strict';
const { pathToFileURL } = require('node:url');
const { createRequire } = require('node:module');
const require2 = createRequire(__filename);
const bin = require2.resolve('@deepseek-ai/dsh/lib/bin.js');
import(pathToFileURL(bin).href).catch((err) => {
  console.error(err);
  process.exit(1);
});
`

// writeBridge 向闭包写入 dsh-bridge 伪包。
func writeBridge(nmDir string) error {
	dir := filepath.Join(nmDir, bridgeName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(bridgePkgJSON), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "index.cjs"), []byte(bridgeIndex), 0o644)
}

// SeaExe 返回 SEA 可执行文件路径。
func SeaExe(ws string, cfg *config.Config) string {
	exe := filepath.Join(config.SeaDir(ws, cfg), "bin", "dsh")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	return exe
}

// Build 执行一次完整的 SEA 打包，返回 SEA 可执行文件路径。
// 全部产物（暂存、工具链、可执行）都位于工作区 target/ 下。
func Build(ws string, cfg *config.Config, skipInstall bool) (string, error) {
	profileDir, err := profile.Ensure(ws, skipInstall)
	if err != nil {
		return "", err
	}
	if _, err := tools.Ensure(ws); err != nil {
		return "", err
	}

	staging := config.SeaDir(ws, cfg)
	if err := fsutil.RemoveAll(staging); err != nil {
		return "", fmt.Errorf("clean staging: %w", err)
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return "", fmt.Errorf("mkdir staging: %w", err)
	}

	// 1) 闭包：profile node_modules → sea/node_modules（解引用；同时过滤
	// 与当前平台无关的原生二进制，见 fsutil.NativeSkip）。
	nmSrc := filepath.Join(profileDir, "node_modules")
	nmDst := filepath.Join(staging, "node_modules")
	if err := fsutil.CopyDirDeref(nmSrc, nmDst, skipEntries, fsutil.NativeSkip); err != nil {
		return "", fmt.Errorf("copy closure: %w", err)
	}

	// 2) dsh-bridge：向闭包写入 CJS 桥（运行时从 exe 旁闭包解析加载）。
	if err := writeBridge(nmDst); err != nil {
		return "", fmt.Errorf("write dsh-bridge: %w", err)
	}

	// 3) 资源：dsh 主包的 config/ 与 package.json。
	dshPkg := profile.DshPkgDir(profileDir)
	if err := fsutil.CopyDir(filepath.Join(dshPkg, "config"), filepath.Join(staging, "config")); err != nil {
		return "", fmt.Errorf("copy config: %w", err)
	}
	if err := fsutil.CopyFile(filepath.Join(dshPkg, "package.json"), filepath.Join(staging, "package.json")); err != nil {
		return "", fmt.Errorf("copy package.json: %w", err)
	}

	// 4) 生成打包入口与配置（templates/ 内嵌）。
	entries, err := templates.ReadDir("templates")
	if err != nil {
		return "", fmt.Errorf("read embedded templates: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := templates.ReadFile("templates/" + e.Name())
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(staging, e.Name()), data, 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", e.Name(), err)
		}
	}

	// 5) 构建。
	if err := tools.Run(ws, staging, "tsdown", "-c", "tsdown.config.mjs"); err != nil {
		return "", err
	}

	// 校验产物不含未内联的裸导入：SEA blob 内只能解析 node 内置模块；
	// 闭包缺包时 bundler 会把解析不到的依赖原样保留，运行期启动即抛
	// ERR_UNKNOWN_BUILTIN_MODULE。此校验让坏产物在构建期暴露。
	if err := checkBareImports(filepath.Join(staging, "dist", "sea-entry.mjs")); err != nil {
		return "", err
	}

	exe := SeaExe(ws, cfg)
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("SEA 产物缺失: %w", err)
	}
	return exe, nil
}

// bareImportRE 匹配产物里的 ESM 模块导入 specifier：`from "..."`、副作用
// `import "..."` 与动态 `import("...")`（bundler 产物用双引号，相对/绝对
// 导入已全部内联，留下的裸 specifier 即缺包）。CJS `require("...")` 不
// 在此列：函数体动态 require 走 createRequire(import.meta.url)，运行时从
// 可执行文件旁的外部闭包解析（原生模块外置的设计），与 ESM 顶层导入
// （SEA blob 编译期即抛 ERR_UNKNOWN_BUILTIN_MODULE）不同。
var bareImportRE = regexp.MustCompile(`(?:from\s+"([^"]+)")|(?:import\s+"([^"]+)")|(?:import\("([^"]+)"\))`)

// checkBareImports 报告 bundle 中非 node: 前缀的 ESM 裸导入（去重）。
func checkBareImports(bundlePath string) error {
	data, err := os.ReadFile(bundlePath)
	if err != nil {
		return err
	}
	var bad []string
	seen := map[string]bool{}
	for _, m := range bareImportRE.FindAllStringSubmatch(string(data), -1) {
		spec := ""
		for _, g := range m[1:] {
			if g != "" {
				spec = g
				break
			}
		}
		if spec == "" || strings.HasPrefix(spec, "node:") || seen[spec] {
			continue
		}
		seen[spec] = true
		bad = append(bad, spec)
	}
	if len(bad) > 0 {
		return fmt.Errorf("SEA bundle 含未内联的 ESM 依赖导入 %v（闭包缺包？检查 %s）",
			bad, filepath.Join(filepath.Dir(bundlePath), "node_modules"))
	}
	return nil
}
