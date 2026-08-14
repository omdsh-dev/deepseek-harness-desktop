// Package shellsrc 嵌入构建壳（Wails v3）所需的最小 Go 模块源码，让 CLI
// 二进制脱离源码树运行：go install 后 bundle 也能在运行时解出源码并
// `go build .` 壳，不依赖仓库 checkout。
//
// _src/ 是完整的"壳专用模块根"（go.mod.txt + 壳源码平铺 + server/），
// 解出后可直接构建（见 cli.materializeShellSrc）：
//
//	go.mod.txt → go.mod（模块定义，精简到只含壳依赖）
//	main.go 等 → 模块根 package main（壳入口）
//	server/    → 模块根包（壳内 import 固定为 .../server）
//
// 布局由 scripts/sync-shellsrc.sh 同步生成：在 _src 内跑 `go mod tidy`
// 把 go.mod 精简到只含壳依赖（去掉 tool 指令与 CLI 专用依赖
// gitignore/oksvg/rasterx/x-image）。go.mod 以 .txt 后缀存放——Go 把
// 任何含 go.mod 的目录视为模块根，embed 跨模块边界被禁止（all:_src
// 会因目录内 go.mod 报 different module）。go.sum 不嵌入：运行时构建用
// GOFLAGS=-mod=mod，依赖已在 GOMODCACHE（CLI 编译时下载），go 自动补全。
//
// 目录以下划线开头：Go 工具链忽略 _ 开头目录，`go build ./...` /
// `go test ./...` 不会把副本当主模块的包编译（embed 是文件系统层，
// 不受包扫描影响）。修改壳源码或 go.mod 后必须重新同步
// （just sync-shell-src / scripts/sync-shellsrc.sh）。
package shellsrc

import "embed"

//go:embed all:_src
var FS embed.FS
