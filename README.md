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
cd examples/custom && deepseek-harness-desktop plugin add @foo/bar   # 向工作区加插件（代理 dsh plugin add）
```

- `dev [<workspace>]` — 基于工作区直接起 `dsh web` 并打开浏览器页面
  （Ctrl+C 退出）。缺省当前目录；目录还不是工作区（缺 package.json）时
  自动从模板创建工程文件并安装依赖，可在任意目录起步。运行时 DSH_HOME
  为工作区本地临时目录 `.dsh-store`（每次 dev 重建，不污染打包应用使用
  的全局数据目录），`profiles/web` 符号链接指向工作区，工作区配置修改
  直接生效
- `bundle <workspace>` — 打包为平台应用，产物在 `target/<name>/` 下。默认基于
  工作区内容 hash 增量：输入无变化时直接复用上次产物；`--force` 忽略缓存
  全新打包；`--install` 打包后安装（macOS `/Applications`、Linux XDG data +
  `.desktop`、Windows `%LOCALAPPDATA%\Programs`）
- `plugin add <package...>` — 代理 `dsh plugin add`：在工作区跑 `pnpm add`，
  并把声明 `dsh.bundle` 的依赖自动加入 `dsh.profile.bundles`。不安装到全局
  `~/.dsh`，只改工作区（默认当前目录，`--workspace=<path>` 指定其他工作区）。
  与官方 dsh 的 reconcile 语义一致：bundle 包自动入层，普通依赖只警告不入层

工作区是拍平的 desktop 定义：`package.json`（name/version/dependencies +
`dsh.profile.bundles` + `dsh.desktop`）+ `cordis.patch.yml`（patch 层）+
`pnpm-workspace.yaml` + `.npmrc`。可先在工作区 `pnpm install`，再用官方
dsh 验证（见 [docs/workspace.md](docs/workspace.md)）。

构建依赖：`go`、`node`、`pnpm`（[mise.toml](mise.toml) 管理）。仓库内开发
用 `go tool deepseek-harness-desktop`（go.mod `tool` 指令注册，无需
`go build`）。

- [docs/workspace.md](docs/workspace.md) — 工作区结构与「先验证，再打包」流程
- [docs/architecture.md](docs/architecture.md) — 产物、架构与构建原理
