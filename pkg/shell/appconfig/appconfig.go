// Package appconfig 解析打包后桌面壳的运行时配置（appconfig.json）。
package appconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config 是 CLI 打包时写入壳同目录 appconfig.json 的运行时配置。
type Config struct {
	Name    string `json:"name"`    // 应用名（窗口标题、XDG 数据目录名）
	ID      string `json:"id"`      // bundle 标识（macOS CFBundleIdentifier）
	Version string `json:"version"` // 应用版本号
	Window  struct {
		Width     int `json:"width"`
		Height    int `json:"height"`
		MinWidth  int `json:"minWidth"`
		MinHeight int `json:"minHeight"`
	} `json:"window"`
	Profile string `json:"profile"` // 后端 boot 的 dsh profile 名
	DSHHome string `json:"dshHome"` // DSH_HOME 策略：xdg（默认）| env | 绝对路径
}

const appConfigFile = "appconfig.json"

// Default 返回默认配置：1280x800、profile web、DSH_HOME 按 XDG。
func Default() Config {
	var c Config
	c.Name = "dsh-desktop"
	c.Window.Width = 1280
	c.Window.Height = 800
	c.Window.MinWidth = 800
	c.Window.MinHeight = 600
	c.Profile = "web"
	c.DSHHome = "xdg"
	return c
}

// Load 读取壳同目录 appconfig.json；缺失或解析失败回退默认值。
func Load(exeDir string) Config {
	cfg := Default()
	raw, err := os.ReadFile(filepath.Join(exeDir, appConfigFile))
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "[dsh-desktop] 解析 %s 失败（使用默认配置）: %v\n", appConfigFile, err)
		return cfg
	}
	if cfg.Name == "" {
		cfg.Name = "dsh-desktop"
	}
	if cfg.Profile == "" {
		cfg.Profile = "web"
	}
	if cfg.DSHHome == "" {
		cfg.DSHHome = "xdg"
	}
	if cfg.Window.Width == 0 {
		cfg.Window.Width = 1280
	}
	if cfg.Window.Height == 0 {
		cfg.Window.Height = 800
	}
	if cfg.Window.MinWidth == 0 {
		cfg.Window.MinWidth = 800
	}
	if cfg.Window.MinHeight == 0 {
		cfg.Window.MinHeight = 600
	}
	return cfg
}
