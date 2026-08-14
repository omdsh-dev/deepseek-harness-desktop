// Package sea 把已安装的 dsh profile 打包为 SEA（Single Executable
// Application）单文件后端。
//
// 打包完全在 target/<name>/sea/ 内完成：
//  1. 闭包：构建出的 profile 的 node_modules（扁平布局）解引用 symlink
//     复制为 sea/node_modules（SEA 运行时 cordis 插件动态 import 的解析根）；
//  2. 资源：@deepseek-ai/dsh 包的 config/ 与 package.json 复制为
//     sea/config 与 sea/package.json（dsh 代码经 `new URL('../config/…',
//     import.meta.url)` 从可执行文件上一级解析）；
//  3. 生成 sea-entry.mjs（dsh CLI 入口）与 tsdown.config.mjs（bundler +
//     node --build-sea）；
//  4. 调用工具链 tsdown 产出 sea/bin/dsh。
package sea

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/dsh-external/deepseek-harness-desktop/internal/config"
	"github.com/dsh-external/deepseek-harness-desktop/internal/fsutil"
	"github.com/dsh-external/deepseek-harness-desktop/internal/profile"
	"github.com/dsh-external/deepseek-harness-desktop/internal/tools"
)

// 复制时排除的 node_modules 簿记条目（非包实体）。
var skipEntries = map[string]bool{
	".store":        true,
	".nub":          true,
	".modules.yaml": true,
}

// entryMJS 是 SEA 打包入口：直接启动 dsh CLI（lib/bin.js 及其静态依赖在
// 构建期由 bundler 内联进 SEA blob）。不集成 tsx —— 正式打包产物不启用
// dsh 的源码模式路径（需要 tsx transform hook 的未打包分支）。
const entryMJS = `// dsh SEA 打包入口（由 deepseek-harness-desktop CLI 生成）。
await import('./node_modules/@deepseek-ai/dsh/lib/bin.js')
`

// tsdownConfigMJS 是 SEA 打包配置：内联 entry 及其静态依赖，用 Node 的
// --build-sea 生成单文件可执行。原生模块与运行时资源（config、frontend
// dist、插件闭包 node_modules）外置在可执行文件旁。闭包即本目录
// node_modules（扁平布局），bundler 直接从本地解析，无需 alias 映射。
const tsdownConfigMJS = `// dsh SEA 打包配置（由 deepseek-harness-desktop CLI 生成）。
export default {
  entry: ['sea-entry.mjs'],
  format: ['esm'],
  platform: 'node',
  target: 'es2024',
  dts: false,
  clean: true,
  // 普通 bundle（无用途的中间产物）也收拢到 dist/ 下，避免污染目录根。
  outDir: 'dist',
  deps: { alwaysBundle: /./ },
  exe: {
    fileName: 'dsh',
    // 可执行文件必须落在深层目录：代码里 new URL('../config/…', import.meta.url)
    // 从 exe 所在目录的上一级解析运行时资源。
    outDir: 'bin',
    seaConfig: {
      execArgv: ['--expose-internals'],
    },
  },
}
`

// SeaExe 返回 SEA 可执行文件路径。
func SeaExe(root string, cfg *config.Config) string {
	exe := filepath.Join(config.SeaDir(root, cfg), "bin", "dsh")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	return exe
}

// Build 执行一次完整的 SEA 打包，返回 SEA 可执行文件路径。
func Build(root, ws string, cfg *config.Config, skipInstall bool) (string, error) {
	profileDir, err := profile.Ensure(root, ws, cfg, skipInstall)
	if err != nil {
		return "", err
	}
	if _, err := tools.Ensure(root); err != nil {
		return "", err
	}

	staging := config.SeaDir(root, cfg)
	if err := os.RemoveAll(staging); err != nil {
		return "", fmt.Errorf("clean staging: %w", err)
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return "", fmt.Errorf("mkdir staging: %w", err)
	}

	// 1) 闭包：profile node_modules → sea/node_modules（解引用）。
	nmSrc := filepath.Join(profileDir, "node_modules")
	nmDst := filepath.Join(staging, "node_modules")
	if err := fsutil.CopyDirDeref(nmSrc, nmDst, skipEntries); err != nil {
		return "", fmt.Errorf("copy closure: %w", err)
	}

	// 2) 资源：dsh 主包的 config/ 与 package.json。
	dshPkg := profile.DshPkgDir(profileDir)
	if err := fsutil.CopyDir(filepath.Join(dshPkg, "config"), filepath.Join(staging, "config")); err != nil {
		return "", fmt.Errorf("copy config: %w", err)
	}
	if err := fsutil.CopyFile(filepath.Join(dshPkg, "package.json"), filepath.Join(staging, "package.json")); err != nil {
		return "", fmt.Errorf("copy package.json: %w", err)
	}

	// 3) 生成打包入口与配置。
	if err := os.WriteFile(filepath.Join(staging, "sea-entry.mjs"), []byte(entryMJS), 0o644); err != nil {
		return "", fmt.Errorf("write sea-entry.mjs: %w", err)
	}
	if err := os.WriteFile(filepath.Join(staging, "tsdown.config.mjs"), []byte(tsdownConfigMJS), 0o644); err != nil {
		return "", fmt.Errorf("write tsdown.config.mjs: %w", err)
	}

	// 4) 构建。
	if err := tools.Run(root, staging, "tsdown", "-c", "tsdown.config.mjs"); err != nil {
		return "", err
	}

	exe := SeaExe(root, cfg)
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("SEA 产物缺失: %w", err)
	}
	return exe, nil
}
