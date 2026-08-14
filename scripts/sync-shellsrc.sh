#!/usr/bin/env bash
# 把壳源码与精简 go.mod 同步到 CLI 的 embed 副本（internal/cli/shellsrc/_src）。
#
# 壳由两部分组成：模块根 package main（internal/shell/*.go + landing.html，
# 平铺到 _src）与 server/ 包（模块根目录）。_src 因此是完整的"壳专用
# 模块根"：go.mod + package main + server/，解出后 `go build .` 即得壳
# 二进制。壳内 import 固定为 .../server（server 位于模块根 server/）。
#
# go.mod 在 _src 内直接 go mod tidy，精简到只含壳依赖（去掉 tool 指令与
# CLI 专用依赖 gitignore/oksvg/rasterx/x-image），完成后重命名为 go.mod.txt
# （embed 规则：目录含 go.mod 会触发模块边界，见 shellsrc 包注释）。
#
# 构建/发布 CLI 前必须执行：CLI 二进制在运行时用这份副本解出并
# go build 壳，脱离源码树（go install 后 bundle）。改了壳源码或 go.mod
# 后先跑 just sync-shell-src。
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
src="$root/internal/cli/shellsrc/_src"

rm -rf "$src"
mkdir -p "$src"
cp -R "$root/internal/shell/." "$src/"
cp -R "$root/server" "$src/"
# tool 指令引用 CLI 主包，tidy 上下文没有它；删掉后 tidy 只保留壳的依赖。
grep -v '^tool ' "$root/go.mod" > "$src/go.mod"
(cd "$src" && go mod tidy)
# go.sum 不嵌入（运行时构建用 GOFLAGS=-mod=mod 自动补全）。
rm -f "$src/go.sum"
mv "$src/go.mod" "$src/go.mod.txt"
