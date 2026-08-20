// Package shell 是打包后桌面壳的构建输入（源码 + 启动页），被 CLI 以
// go:embed 内嵌：运行时解出为独立 Go module（go.mod 由 CLI 动态生成），
// go build 出壳二进制。源码即本目录各子包，壳构建与主模块共享同一份代码，
// 无内嵌副本同步问题。
package shell

import "embed"

//go:embed all:appconfig all:dshhome all:server all:supervise all:cmd
var FS embed.FS
