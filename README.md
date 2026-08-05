# deepseek-harness-desktop

为 [deepseek-harness](deepseek-harness/) 打包桌面应用：Wails v3 壳
（`dsh-shell`）+ SEA 单文件后端（`dsh-server`，内嵌 node 的 `dsh web`），
Web 界面内嵌在 Webview 窗口里，不依赖外部浏览器。支持 macOS 与 Linux。

## 快速开始

依赖（[mise](https://mise.jdx.dev) 管理）：`just`、`go`、`nub`，见 [mise.toml](./mise.toml)。

macOS：

```sh
just sea                 # 构建 SEA 单文件可执行（含外置资源）
just build-macos-app     # 完整构建（SEA + Wails 壳 + 图标 + 组装 target/DSH.app）
just install-macos-app   # 构建并安装到 /Applications/DSH.app
just run-macos-app       # 运行 target/DSH.app
```

Linux（须在 Linux 主机上执行，见下）：

```sh
just build-linux-app     # 完整构建（SEA + Wails 壳 + 组装 target/linux/DSH + DSH.tar.gz）
```

产物统一在 `target/`：`target/sea/`（SEA 产物与资源）、`target/DSH.app`（macOS 应用）、
`target/dsh.icns`（macOS 图标）、`target/linux/`（Linux 应用）。

## 目录结构

- `apps/dsh-desktop/` — Wails 壳（Go），桌面应用入口与后端守护，详见 [README](apps/dsh-desktop/README.md)
- `scripts/` — 构建脚本（TypeScript + [zx](https://google.github.io/zx/)）：
  `sea-materialize.mts`（SEA 运行时资源实体化）、`make-macos-app.mts`（macOS 组装）、
  `make-linux-app.mts`（Linux 组装）、`icon.mts`（macOS 图标生成）
- `deepseek-harness/` — 上游源代码（已 gitignore，**严禁修改**，见 [AGENTS.md](./AGENTS.md)）

## 构建说明

- `just sea` 依赖 `build-libs`：编译上游 lib（tsc + tsdown），并 `vite build` 产出前端
  `apps/web/dist`。`dsh web` 启动时经 `require.resolve('@deepseek-ai/dsh-frontend/dist/index.html')`
  解析该产物——缺失时后端启动即报 `dsh: frontend dist not built` 退出，表现为窗口停在启动页/空白。
- macOS 构建命令带 `-macos-app` 后缀；Linux 构建命令带 `-linux-app` 后缀；`default` 列出全部 recipe（`just --list`）。
- **Linux 构建须在 Linux 主机上执行**：SEA（Node `--build-sea`）与 Wails 壳（cgo WebKitGTK）
  均不支持交叉编译，macOS 上产出的 `target/sea` 不能用于 Linux。Linux 主机需安装
  WebKitGTK 开发库（Wails 壳运行依赖）；图标由 sharp（libvips + librsvg）渲染，随 npm 预编译。
- Linux 产物 `target/linux/DSH/`：`bin/dsh-shell`（壳）、`bin/dsh-server`（SEA）、
  `config/`、`node_modules/`、`package.json`（资源，dsh-server 从 bin 上一级解析）、
  `share/icons/dsh.png`（图标）；另有 `DSH.tar.gz` 归档。

## 环境变量

- 构建期：`DEEPSEEK_HARNESS_REPO` — `just sync` 拉取上游 deepseek-harness 的仓库地址
- 运行时（壳 `dsh-shell`）：`DSH_APP_WORKSPACE`（工作目录）、`DSH_APP_PORT`（后端端口），
  详见 [dsh-desktop README](apps/dsh-desktop/README.md)
- 透传给后端（`dsh-server`）：`DSH_HOME`（`$DSH_HOME/config.yaml` 配置覆盖、`$DSH_HOME/.env` 凭据）、
  `DEEPSEEK_API_KEY` / `DEEPSEEK_BASE_URL`（LLM 凭据）。壳启动后端前 source 用户 shell 配置，
  使这些变量从终端环境继承；也可放调用目录 `.env`
