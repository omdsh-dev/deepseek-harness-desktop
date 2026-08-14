// Package profile 把工作区的拍平 desktop 定义装配为 DSH_HOME 布局并安装
// 依赖闭包。
//
// 装配目标（构建目录，全部产物在 target/ 下）：
//
//	target/<name>/dsh-home/            = 构建出的 DSH_HOME（= bundle 种子）
//	  profiles/web/package.json        ← 工作区 package.json（bundles + deps）
//	  profiles/web/cordis.patch.yml    ← 工作区 cordis.patch.yml（patch 层）
//	  profiles/web/pnpm-workspace.yaml   nodeLinker: hoisted + autoInstallPeers
//	  profiles/web/.npmrc                registry + 工作区本地 store
//	  profiles/web/node_modules/         nub install 安装的依赖闭包
//
// dsh 的 boot 固定从 $DSH_HOME/profiles/<name>/ 读 profile（loadProfile），
// 故拍平的工作区内容在此装配回 profiles/web/ 布局；desktop 只有一个
// profile（web），无需泛化多 profile。
package profile

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dsh-external/deepseek-harness-desktop/internal/config"
	"github.com/dsh-external/deepseek-harness-desktop/internal/fsutil"
)

//go:embed all:templates
var templates embed.FS

// 需要放行构建脚本的包（原生模块，nub 默认忽略构建脚本）。
var allowBuilds = []string{
	"@deepseek-ai/dsh-subprocess-local",
	"@google/genai",
	"esbuild",
	"koffi",
	"node-pty",
	"protobufjs",
}

// 工程文件模板（.npmrc / pnpm-workspace.yaml）以 go:embed 内嵌在
// templates/ 下：.npmrc 含 registry 映射（@deepseek-ai 官方 npm +
// @morlay GitHub npm）与构建目录本地 store；pnpm-workspace.yaml 的
// autoInstallPeers=true 保证 dsh 核心 peer 依赖（cordis-plugin-group 等）
// 进闭包（缺它们时 SEA 打包会把裸包名 import 留在 blob 里，启动即崩）。

// ProfileDir 返回构建出的 profile 目录（target/<name>/dsh-home/profiles/web）。
func ProfileDir(root string, cfg *config.Config) string {
	return filepath.Join(config.DSHHomeDir(root, cfg), "profiles", config.ProfileName)
}

// Installed 报告 profile 是否已安装（node_modules 中存在 dsh 主包）。
func Installed(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "node_modules", "@deepseek-ai", "dsh", "package.json"))
	return err == nil
}

// Assemble 把工作区的拍平定义装配为 DSH_HOME 布局（幂等）：package.json
// 注入原生包信任 allowBuilds，profile patch 层原样复制。settings.yaml 等
// 用户运行时数据不属于工作区，由用户在应用内配置生成。
func Assemble(root, ws string, cfg *config.Config) error {
	profileDir := ProfileDir(root, cfg)
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", profileDir, err)
	}

	// profile 清单：复用工作区 package.json，注入 allowBuilds。
	manifestPath := filepath.Join(ws, "package.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}
	var m struct {
		AllowBuilds map[string]bool `json:"allowBuilds"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("parse %s: %w", manifestPath, err)
	}
	if len(m.AllowBuilds) == 0 {
		m.AllowBuilds = map[string]bool{}
		for _, p := range allowBuilds {
			m.AllowBuilds[p] = true
		}
		var full map[string]any
		if err := json.Unmarshal(raw, &full); err != nil {
			return fmt.Errorf("parse %s: %w", manifestPath, err)
		}
		full["allowBuilds"] = m.AllowBuilds
		out, err := json.MarshalIndent(full, "", "  ")
		if err != nil {
			return fmt.Errorf("encode %s: %w", manifestPath, err)
		}
		raw = append(out, '\n')
	}
	if err := os.WriteFile(filepath.Join(profileDir, "package.json"), raw, 0o644); err != nil {
		return fmt.Errorf("write package.json: %w", err)
	}

	// 工程文件（templates/ 内嵌）。
	entries, err := templates.ReadDir("templates")
	if err != nil {
		return fmt.Errorf("read embedded templates: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := templates.ReadFile("templates/" + e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(profileDir, e.Name()), data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", e.Name(), err)
		}
	}
	if err := fsutil.CopyFile(filepath.Join(ws, "cordis.patch.yml"), filepath.Join(profileDir, "cordis.patch.yml")); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("copy cordis.patch.yml: %w", err)
		}
		// 工作区未提供 patch 层：写入 dsh 官方空模板。
		if err := os.WriteFile(filepath.Join(profileDir, "cordis.patch.yml"), []byte("[]\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Install 在 profile 目录运行 nub install（增量；已有安装时快速收敛）。
// nub 从 PATH 解析（mise 管理）。
func Install(dir string, skip bool) error {
	if skip {
		return nil
	}
	cmd := exec.Command("nub", "install")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nub install（%s）: %w", dir, err)
	}
	return nil
}

// Ensure 确保构建出的 profile 已装配并安装，返回 profile 目录。
func Ensure(root, ws string, cfg *config.Config, skipInstall bool) (string, error) {
	if err := Assemble(root, ws, cfg); err != nil {
		return "", err
	}
	dir := ProfileDir(root, cfg)
	if !Installed(dir) && !skipInstall {
		if err := Install(dir, false); err != nil {
			return "", err
		}
	}
	if !Installed(dir) {
		return "", fmt.Errorf("profile 未安装（%s/node_modules/@deepseek-ai/dsh 缺失）；先执行 bundle", dir)
	}
	return dir, nil
}

// DshPkgDir 返回已安装的 @deepseek-ai/dsh 主包目录。
func DshPkgDir(profileDir string) string {
	return filepath.Join(profileDir, "node_modules", "@deepseek-ai", "dsh")
}

// Version 返回已安装 dsh 的版本号（读主包 package.json）。
func Version(profileDir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(DshPkgDir(profileDir), "package.json"))
	if err != nil {
		return "", err
	}
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	return m.Version, nil
}
