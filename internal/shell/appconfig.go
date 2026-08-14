package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// appConfig 是 CLI 打包时写入壳同目录 appconfig.json 的运行时配置。
// 所有字段都有默认值，缺省 appconfig.json 时壳按默认行为运行。
type appConfig struct {
	// Name 是应用名（窗口标题、XDG 数据目录名）。
	Name string `json:"name"`
	// ID 是 bundle 标识（macOS CFBundleIdentifier）。
	ID string `json:"id"`
	// Version 是应用版本号。
	Version string `json:"version"`
	Window  struct {
		Width     int `json:"width"`
		Height    int `json:"height"`
		MinWidth  int `json:"minWidth"`
		MinHeight int `json:"minHeight"`
	} `json:"window"`
	// Profile 是后端 boot 的 dsh profile 名。
	Profile string `json:"profile"`
	// DSHHome 是 DSH_HOME 解析策略：xdg（默认）| env | 绝对路径。
	DSHHome string `json:"dshHome"`
}

const appConfigFile = "appconfig.json"

// defaultAppConfig：1280x800、profile web、DSH_HOME 按 XDG。
func defaultAppConfig() appConfig {
	var c appConfig
	c.Name = "dsh-desktop"
	c.Window.Width = 1280
	c.Window.Height = 800
	c.Window.MinWidth = 800
	c.Window.MinHeight = 600
	c.Profile = "web"
	c.DSHHome = "xdg"
	return c
}

// loadAppConfig 读取壳可执行文件同目录的 appconfig.json；缺失或解析失败
// 时回退默认值（解析失败仅告警，不阻断启动）。
func loadAppConfig(exeDir string) appConfig {
	cfg := defaultAppConfig()
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
