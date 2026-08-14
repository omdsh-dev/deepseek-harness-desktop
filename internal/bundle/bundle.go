// Package bundle 把 SEA 产物、壳二进制与构建出的 DSH_HOME 装配为平台
// 桌面应用与开发布局。全部产物在 target/<name>/ 下：
//
//	macOS   target/<name>/<Name>.app/Contents/{MacOS,Resources,config,node_modules,package.json,dsh-home,Info.plist}
//	Linux   target/<name>/linux/<Name>/{bin,config,node_modules,package.json,dsh-home,share/icons/hicolor}
//	Windows target/<name>/windows/<Name>/{bin,config,node_modules,package.json,dsh-home,dsh.ico}
//	dev     target/<name>/dev/{bin,config,node_modules,package.json,dsh-home}
//
// dsh-home 是打包进应用的 DSH_HOME 种子（profiles/web/ 等，由 profile 包
// 构建），壳在运行时按 appconfig 的 dshHome 策略落位
// （xdg 数据目录 / 固定路径 / 继承环境）。
package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/omdsh-dev/deepseek-harness-desktop/internal/config"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/fsutil"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/gitignore"
)

// appConfig 与 internal/shell 的 appconfig.json 结构一致（壳读取）。
type appConfig struct {
	Name    string `json:"name"`
	ID      string `json:"id"`
	Version string `json:"version"`
	Window  struct {
		Width     int `json:"width"`
		Height    int `json:"height"`
		MinWidth  int `json:"minWidth"`
		MinHeight int `json:"minHeight"`
	} `json:"window"`
	Profile string `json:"profile"`
	DSHHome string `json:"dshHome"`
}

// seedSkip 是复制 DSH_HOME 种子时排除的名字（安装簿记、工程文件与运行时
// 用户数据；basename 命中即跳过）。构建产物（如 target/）不在此列——由
// 工作区 .gitignore 表达，遵循它排除（见 assembleLayout 的 seedIgnored）。
var seedSkip = map[string]bool{
	".nub-store":          true,
	".store":              true,
	".nub":                true,
	".modules.yaml":       true,
	".npmrc":              true,
	"pnpm-workspace.yaml": true,
	"pnpm-lock.yaml":      true,
	// 运行时用户数据不进种子：settings.yaml、storages/、sessions/ 等由
	// 应用在目标 DSH_HOME 中生成。
	"settings.yaml": true,
	"storages":      true,
	"sessions":      true,
}

// Inputs 是一次装配的全部输入。
type Inputs struct {
	Workspace string // 工作区（target/ 产物根与图标源）
	Cfg       *config.Config
	SeaExe    string // SEA 可执行（sea/bin/dsh）
	ShellBin  string // 壳二进制（go build ./internal/shell 的产物）
}

// AppRoot 返回平台应用的产物根目录（位于工作区 target/ 下）。
func AppRoot(ws string, cfg *config.Config) string {
	build := config.BuildDir(ws, cfg)
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(build, cfg.Name+".app")
	case "linux":
		return filepath.Join(build, "linux", cfg.Name)
	case "windows":
		return filepath.Join(build, "windows", cfg.Name)
	}
	return filepath.Join(build, "app")
}

// BinNames 返回壳与后端文件名（平台相关扩展名）。
func BinNames() (shell, server string) {
	if runtime.GOOS == "windows" {
		return "dsh-shell.exe", "dsh-server.exe"
	}
	return "dsh-shell", "dsh-server"
}

// assembleLayout 装配平台无关的公共布局（bin/ + 资源 + 种子），返回 bin
// 目录。appRoot 由调用方先清理。withSeed=false 时不复制 DSH_HOME 种子
// （dev 布局：运行时 home 由 CLI 单独构造）。
func assembleLayout(in Inputs, appRoot string) (string, error) {
	binDir := filepath.Join(appRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}

	// 壳与 SEA 后端。
	shellName, serverName := BinNames()
	if err := fsutil.CopyFile(in.ShellBin, filepath.Join(binDir, shellName)); err != nil {
		return "", fmt.Errorf("copy shell: %w", err)
	}
	if err := fsutil.CopyFile(in.SeaExe, filepath.Join(binDir, serverName)); err != nil {
		return "", fmt.Errorf("copy dsh-server: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(binDir, serverName), 0o755); err != nil {
			return "", err
		}
	}

	// 壳运行时配置。
	if err := writeAppConfig(binDir, in.Cfg); err != nil {
		return "", fmt.Errorf("write appconfig.json: %w", err)
	}

	// SEA 运行时资源：config/、node_modules/、package.json（从 staging 复制，
	// dsh-server 从可执行文件上一级解析）。
	staging := config.SeaDir(in.Workspace, in.Cfg)
	for _, name := range []string{"config", "node_modules", "package.json"} {
		src := filepath.Join(staging, name)
		if err := fsutil.CopyDir(src, filepath.Join(appRoot, name)); err != nil {
			return "", fmt.Errorf("copy %s: %w", name, err)
		}
	}

	// DSH_HOME 种子：工作区（profile 拍平内容）→ appRoot/dsh-home/profiles/web
	// （解引用：pnpm 安装的 node_modules 是 store 链接）。dsh 运行时固定从
	// $DSH_HOME/profiles/<name> 解析 profile。
	// 种子遵循工作区 .gitignore：被忽略的条目（构建产物、缓存等）不进种子；
	// node_modules 例外——SEA 运行时需要依赖闭包，虽被 git 忽略但必须保留。
	homeRoot := filepath.Join(appRoot, "dsh-home")
	if err := os.MkdirAll(filepath.Join(homeRoot, "profiles"), 0o755); err != nil {
		return "", err
	}
	gi, err := gitignore.Load(in.Workspace)
	if err != nil {
		return "", fmt.Errorf("load .gitignore: %w", err)
	}
	seedIgnored := func(rel string, isDir bool) bool {
		if rel == "node_modules" || strings.HasPrefix(rel, "node_modules/") {
			return false
		}
		return gi.Ignored(rel, isDir)
	}
	if err := fsutil.CopyDirDeref(in.Workspace, filepath.Join(homeRoot, "profiles", config.ProfileName), seedSkip, seedIgnored); err != nil {
		return "", fmt.Errorf("copy dsh-home seed: %w", err)
	}

	return binDir, nil
}

// writeAppConfig 生成壳同目录的 appconfig.json。
func writeAppConfig(binDir string, cfg *config.Config) error {
	var ac appConfig
	ac.Name = cfg.Name
	ac.ID = cfg.Desktop.ID
	ac.Version = cfg.Version
	ac.Window.Width = cfg.Desktop.Window.Width
	ac.Window.Height = cfg.Desktop.Window.Height
	ac.Window.MinWidth = cfg.Desktop.Window.MinWidth
	ac.Window.MinHeight = cfg.Desktop.Window.MinHeight
	ac.Profile = config.ProfileName
	ac.DSHHome = cfg.Desktop.DSHHome
	raw, err := json.MarshalIndent(ac, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(binDir, "appconfig.json"), append(raw, '\n'), 0o644)
}

// Assemble 按当前平台组装应用，返回产物根目录。
func Assemble(in Inputs) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return assembleMacOS(in)
	case "linux":
		return assembleLinux(in)
	case "windows":
		return assembleWindows(in)
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// AssembleDev 组装开发布局（target/<name>/dev），返回 bin 目录。
// dev 不复制 DSH_HOME 种子：运行时 home 由 CLI 构造（profiles/web 指向
// 工作区），用户在工作区的 pnpm install 结果直接可见。
