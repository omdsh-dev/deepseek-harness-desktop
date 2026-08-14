# deepseek-harness-desktop

把 [@deepseek-ai/dsh](https://www.npmjs.com/package/@deepseek-ai/dsh) 的
`--profile web` 与 `cordis.patch.yml` 打包为**独立自定义桌面**的 Go 单命令。
支持 macOS、Linux 与 Windows。

## Quick Start

```sh
go install github.com/omdsh-dev/deepseek-harness-desktop/cmd/deepseek-harness-desktop@latest
```

创建你的工作区（复制本仓库 [examples/custom](examples/custom) 即可起步），
然后：

```sh
deepseek-harness-desktop dev examples/custom        # 基于工作区起 dsh web 并打开浏览器
deepseek-harness-desktop bundle examples/custom     # 打包当前平台的应用（基于工作区 hash 增量）
deepseek-harness-desktop bundle --force examples/custom      # 忽略缓存，全新打包
deepseek-harness-desktop bundle --install examples/custom    # 打包并安装到当前平台
```

- `dev <workspace>` — 基于工作区直接起 `dsh web` 并打开浏览器页面（Ctrl+C 退出）
- `bundle <workspace>` — 打包为平台应用，产物在 `target/<name>/` 下。默认基于
  工作区内容 hash 增量：输入无变化时直接复用上次产物；`--force` 忽略缓存
  全新打包；`--install` 打包后安装（macOS `/Applications`、Linux XDG data +
  `.desktop`、Windows `%LOCALAPPDATA%\Programs`）

工作区是拍平的 desktop 定义：`package.json`（name/version/dependencies +
`dsh.profile.bundles` + `dsh.desktop`）+ `cordis.patch.yml`（patch 层）+
`pnpm-workspace.yaml` + `.npmrc`。可先在工作区 `pnpm install`，再用官方
dsh 验证（见 [docs/workspace.md](docs/workspace.md)）。

构建依赖：`go`、`node`、`pnpm`（[mise.toml](mise.toml) 管理）。仓库内开发
用 `go tool deepseek-harness-desktop`（go.mod `tool` 指令注册，无需
`go build`）。

- [docs/workspace.md](docs/workspace.md) — 工作区结构与「先验证，再打包」流程
- [docs/architecture.md](docs/architecture.md) — 产物、架构与构建原理
