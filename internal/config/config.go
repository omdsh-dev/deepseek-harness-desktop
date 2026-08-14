// Package config 解析工作区配置（全部来自 package.json，无独立配置文件）：
//
//	name / version / dependencies      npm 语义字段，直接复用；
//	dsh.profile.bundles                打包的 cordis bundle 列表；
//	dsh.desktop                        桌面特有配置（id、窗口几何、图标、
//	                                   DSH_HOME 策略）。
//
// 工作区（examples/ 下的目录）是一个拍平的 desktop 定义：package.json +
// cordis.patch.yml（profile patch 层）+ settings.yaml（DSH_HOME 层设置，
// 可选）+ 图标。CLI 构建时把它们装配为 DSH_HOME 布局（profiles/web/）并
// 打包。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ProfileName 是 desktop 唯一支持的 dsh profile 名（拍平布局）。
const ProfileName = "web"

// Window 是桌面窗口的几何配置。
type Window struct {
	Width     int `json:"width"`
	Height    int `json:"height"`
	MinWidth  int `json:"minWidth"`
	MinHeight int `json:"minHeight"`
}

// Desktop 是 desktop 特有配置（package.json 的 dsh.desktop 字段）。
type Desktop struct {
	ID string `json:"id"`
	// Icon 是相对工作区的图标源文件（SVG），缺省不生成图标。
	Icon string `json:"icon"`
	// DSHHome 是运行时 DSH_HOME 策略：
	//   缺省            — xdg.DataHome/<name>（XDG_DATA_HOME 规范）；
	//   env             — 不设置 DSH_HOME，继承环境；
	//   绝对路径         — DSH_HOME 固定为该路径，缺失部分从 bundle 种子补齐。
	DSHHome string `json:"dshHome"`
	Window  Window `json:"window"`
}

// Config 是一份完整的工作区配置。
type Config struct {
	// Name 复用 package.json 的 name（应用名：窗口标题、数据目录、产物目录）。
	Name string
	// Version 复用 package.json 的 version。
	Version string
	// Bundles 复用 package.json 的 dsh.profile.bundles。
	Bundles []string
	// Dependencies 是 profile 的依赖声明（package.json）。
	Dependencies map[string]string
	// Desktop 是 dsh.desktop 特有配置。
	Desktop Desktop
}

// manifest 是工作区 package.json 的最小结构。
type manifest struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Private      bool              `json:"private"`
	Dependencies map[string]string `json:"dependencies"`
	DSH          struct {
		Profile struct {
			Bundles []string `json:"bundles"`
		} `json:"profile"`
		Desktop Desktop `json:"desktop"`
	} `json:"dsh"`
}

// Load 读取并校验工作区配置。
func Load(ws string) (*Config, error) {
	manifestPath := filepath.Join(ws, "package.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w（工作区必须提供 package.json，声明 dsh.profile.bundles 与依赖）", manifestPath, err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", manifestPath, err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("%s: name 不能为空（应用名复用 npm 包名）", manifestPath)
	}
	if len(m.DSH.Profile.Bundles) == 0 {
		return nil, fmt.Errorf("%s: dsh.profile.bundles 不能为空（例如 [\"@deepseek-ai/dsh-base\", \"@deepseek-ai/dsh-web-app\"]）", manifestPath)
	}

	cfg := &Config{
		Name:         m.Name,
		Version:      m.Version,
		Bundles:      m.DSH.Profile.Bundles,
		Dependencies: m.Dependencies,
		Desktop:      m.DSH.Desktop,
	}
	if cfg.Version == "" {
		cfg.Version = "0.0.1"
	}
	if cfg.Desktop.DSHHome == "" {
		cfg.Desktop.DSHHome = "xdg"
	}
	if cfg.Desktop.Window.Width == 0 {
		cfg.Desktop.Window.Width = 1280
	}
	if cfg.Desktop.Window.Height == 0 {
		cfg.Desktop.Window.Height = 800
	}
	if cfg.Desktop.Window.MinWidth == 0 {
		cfg.Desktop.Window.MinWidth = 800
	}
	if cfg.Desktop.Window.MinHeight == 0 {
		cfg.Desktop.Window.MinHeight = 600
	}
	if cfg.Desktop.ID == "" {
		cfg.Desktop.ID = "ai.deepseek." + sanitizeID(cfg.Name)
	}
	return cfg, nil
}

// TargetDir 返回仓库 target/ 目录。
func TargetDir(root string) string {
	return filepath.Join(root, "target")
}

// BuildDir 返回该 desktop 的构建目录（target/<name>/）。
func BuildDir(root string, cfg *Config) string {
	return filepath.Join(TargetDir(root), cfg.Name)
}

// DSHHomeDir 返回构建出的 DSH_HOME 目录（target/<name>/dsh-home）。
func DSHHomeDir(root string, cfg *Config) string {
	return filepath.Join(BuildDir(root, cfg), "dsh-home")
}

// SeaDir 返回 SEA 打包暂存目录（target/<name>/sea）。
func SeaDir(root string, cfg *Config) string {
	return filepath.Join(BuildDir(root, cfg), "sea")
}

func sanitizeID(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r-'A'+'a')
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}
