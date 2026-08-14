// Package profile 管理工作区的依赖闭包（工作区根即 profile 内容）。
//
// 工作区根拍平存放 package.json（bundles + deps）、cordis.patch.yml
// （patch 层）、pnpm-workspace.yaml 与 .npmrc（安装工程文件，随工作区
// 提交）。用户可直接在工作区 pnpm install，依赖闭包落在工作区
// node_modules；CLI 复用这份安装（SEA 闭包 / bundle 种子）。
//
// CLI 兜底：工程文件缺失时从内嵌模板生成（与官方推荐配置一致），保证
// 闭包为扁平布局（SEA 打包直接复制）且原生模块构建脚本被放行。
package profile

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/omdsh-dev/deepseek-harness-desktop/internal/pm"
)

//go:embed all:templates
var templates embed.FS

// Installed 报告闭包是否已安装（工作区 node_modules 中存在 dsh 主包）。
func Installed(ws string) bool {
	_, err := os.Stat(filepath.Join(ws, "node_modules", "@deepseek-ai", "dsh", "package.json"))
	return err == nil
}

// Ensure 确保工程文件齐全（缺失则从模板兜底生成），未安装时在工作区
// 运行 pnpm install。返回工作区路径。
func Ensure(ws string, skipInstall bool) (string, error) {
	dir := ws
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	// 工程文件兜底：工作区通常已提交；缺失时从模板生成。
	entries, err := templates.ReadDir("templates")
	if err != nil {
		return "", fmt.Errorf("read embedded templates: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		target := filepath.Join(dir, e.Name())
		if _, err := os.Stat(target); err == nil {
			continue
		}
		data, err := templates.ReadFile("templates/" + e.Name())
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", e.Name(), err)
		}
	}

	if !Installed(ws) && !skipInstall {
		if err := Install(dir, false); err != nil {
			return "", err
		}
	}
	if !Installed(ws) {
		return "", fmt.Errorf("闭包未安装（%s/node_modules/@deepseek-ai/dsh 缺失）；先在工作区执行 pnpm install 或 bundle", dir)
	}
	return dir, nil
}

// Install 在 profile 目录运行 pnpm install（与 dsh 官方一致；增量，
// 已有安装时快速收敛）。
func Install(dir string, skip bool) error {
	if skip {
		return nil
	}
	cmd, err := pm.Command("install")
	if err != nil {
		return err
	}
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pnpm install（%s）: %w", dir, err)
	}
	return nil
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
