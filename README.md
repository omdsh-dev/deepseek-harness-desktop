# deepseek-harness-desktop

为 [deepseek-harness](deepseek-harness/) 打包桌面应用：Wails v3 壳
（`dsh-shell`）+ SEA 单文件后端（`dsh-server`，内嵌 node 的 `dsh web`），
Web 界面内嵌在 Webview 窗口里，不依赖外部浏览器。支持 macOS、Linux 与 Windows。

## 架构：为什么是三层

桌面应用由三层组成，每一层都是 dsh 运行机制的必然结果：

| 层 | 产物 | 职责 | 为什么需要 |
|---|---|---|---|
| 壳 | `dsh-shell`（Wails v3，Go） | 原生窗口 + WebView；后端进程守护（启动/就绪/退避重启/退出清理） | node 进程不提供原生桌面窗口，壳是应用的唯一入口 |
| 后端 | `dsh-server`（SEA，内嵌 node v26.5.0 的 `dsh web`） | 跑 dsh 的 cordis 插件树，HTTP 伺服前端与 API | dsh 是 node/TypeScript 的 cordis 插件化 harness，插件在运行时按字符串包名 `import()`、依赖 npm 闭包——只能在 node 上运行，Go 无法替代 |
| 前端 | `dsh-frontend`（apps/web 的 vite dist） | 浏览器 UI，由 `dsh web` 经 HTTP 伺服，WebView 加载 | UI 是浏览器应用，且 `dsh web` 经 HTTP 注入 `__DSH_BOOT__` 引导，无法脱离后端直接打开 |

三个关键点：

- **必须依赖 node**：dsh 的 cordis 插件树（TS/ESM/npm 生态）只能在 node 运行时上跑，桌面 app 不能假设用户系统装了 node，因此用 SEA（Node Single Executable Application，`--build-sea`）把 node 内嵌进单文件可执行（v26.5.0）。
- **必须走 HTTP**：`dsh web` 以 HTTP 伺服前端与 API（`httpServer` 服务、`__DSH_BOOT__` 注入），壳的 WebView 加载 `http://127.0.0.1:<port>`，端口由 OS 分配避免冲突。
- **壳必须存在**：node 后端不提供原生窗口；壳承担窗口生命周期、后端守护，并在退出时终止后端进程组，不留孤儿 node。

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

Windows（须在 Windows 主机上执行，见下）：

```sh
just build-windows-app   # 完整构建（SEA + Wails 壳 + 组装 target/windows/DSH + DSH.zip）
```

产物统一在 `target/`：`target/sea/`（SEA 产物与资源）、`target/DSH.app`（macOS 应用）、
`target/dsh.icns`（macOS 图标）+ `target/dsh.iconset/`（macOS 多尺寸 PNG 集 16–1024）、
`target/linux/`（Linux 应用）、`target/windows/`（Windows 应用）。

## 目录结构

- `apps/dsh-desktop/` — Wails 壳（Go），桌面应用入口与后端守护，详见 [README](apps/dsh-desktop/README.md)
- `scripts/` — 构建脚本（TypeScript + [zx](https://google.github.io/zx/)）：
  `sea-materialize.mts`（SEA 运行时资源实体化）、`make-macos-app.mts`（macOS 组装）、
  `make-linux-app.mts`（Linux 组装）、`make-windows-app.mts`（Windows 组装）、
  `icon.mts`（macOS 图标生成）
- `deepseek-harness/` — 上游源代码（已 gitignore，**严禁修改**，见 [AGENTS.md](./AGENTS.md)）

## 构建说明

- `just sea` 依赖 `build-libs`：编译上游 lib（tsc + tsdown），并 `vite build` 产出前端
  `apps/web/dist`。`dsh web` 启动时经 `require.resolve('@deepseek-ai/dsh-frontend/dist/index.html')`
  解析该产物——缺失时后端启动即报 `dsh: frontend dist not built` 退出，表现为窗口停在启动页/空白。
- macOS 构建命令带 `-macos-app` 后缀；Linux 构建命令带 `-linux-app` 后缀；Windows 构建
  命令带 `-windows-app` 后缀；`default` 列出全部 recipe（`just --list`）。
- **Linux 构建须在 Linux 主机上执行**：SEA（Node `--build-sea`）与 Wails 壳（cgo WebKitGTK）
  均不支持交叉编译，macOS 上产出的 `target/sea` 不能用于 Linux。Linux 主机需安装
  WebKitGTK 开发库（Wails 壳运行依赖）；图标由 sharp（libvips + librsvg）渲染，随 npm 预编译。
- Linux 产物 `target/linux/DSH/`：`bin/dsh-shell`（壳）、`bin/dsh-server`（SEA）、
  `config/`、`node_modules/`、`package.json`（资源，dsh-server 从 bin 上一级解析）、
  `share/icons/hicolor/`（freedesktop 多尺寸图标集 16–512 + scalable SVG）；
  另有 `DSH.tar.gz` 归档。
- **Windows 构建须在 Windows 主机上执行**：SEA（Node `--build-sea`）与 Wails 壳
  （WebView2）均不支持交叉编译，macOS 上产出的 `target/sea` 不能用于 Windows。壳以
  GUI 子系统构建（`-ldflags "-H=windowsgui"`，启动不弹控制台黑窗），后端以
  `CREATE_NO_WINDOW` 标志启动；退出收口用 Job Object（`KILL_ON_JOB_CLOSE`）按作业树
  终止后端全树（Windows 无 POSIX 信号，SEA node 收不到优雅 SIGTERM）。
  Windows 10 1803+ 自带 WebView2 运行时与 bsdtar（打包 `DSH.zip` 用，`-a` 按后缀选 zip）。
- Windows 产物 `target/windows/DSH/`：`bin/dsh-shell.exe`（壳）、`bin/dsh-server.exe`（SEA）、
  `config/`、`node_modules/`、`package.json`（资源，dsh-server 从 bin 上一级解析）、
  `dsh.ico`（多尺寸 PNG 内嵌图标，纯 node 生成）；另有 `DSH.zip` 归档。

## 环境变量

- 构建期：`DEEPSEEK_HARNESS_REPO` — `just sync` 拉取上游 deepseek-harness 的仓库地址；
  可选 `DEEPSEEK_HARNESS_REPO_BRANCH` — 设置后 `just sync` 以 `-b <分支>` 拉取指定分支
- 运行时（壳 `dsh-shell`）：`DSH_APP_WORKSPACE`（工作目录）、`DSH_APP_PORT`（后端端口），
  详见 [dsh-desktop README](apps/dsh-desktop/README.md)
- 透传给后端（`dsh-server`）：`DSH_HOME`（`$DSH_HOME/config.yaml` 配置覆盖、`$DSH_HOME/.env` 凭据）、
  `DEEPSEEK_API_KEY` / `DEEPSEEK_BASE_URL`（LLM 凭据）。壳启动后端前 source 用户 shell 配置，
  使这些变量从终端环境继承；也可放调用目录 `.env`
