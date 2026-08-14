# 产物、架构与构建原理

## 产物

`bundle` 依次完成：pnpm 安装依赖闭包（扁平布局、`autoInstallPeers` 保证
dsh 核心 peer 依赖完整）→ SEA 打包（内嵌 node 的 `dsh --profile web`
单文件后端）→ Wails 壳构建 → 平台组装。

| 平台 | 产物 |
|---|---|
| macOS | `target/<name>/<Name>.app`（Info.plist、icns） |
| Linux | `target/<name>/linux/<Name>/` + `tar.gz`（hicolor 图标集） |
| Windows | `target/<name>/windows/<Name>/` + `zip`（ico） |

`bundle` 只支持本机平台（SEA 与 Wails 壳均不支持交叉编译），
`--platform=os/arch` 用于显式声明并校验。

## 架构

桌面应用由三层组成，每一层都是 dsh 运行机制的必然结果：

| 层 | 产物 | 职责 | 为什么需要 |
|---|---|---|---|
| 壳 | `dsh-shell`（Wails v3，Go） | 原生窗口 + WebView；后端进程守护（启动/就绪/退避重启/退出清理） | node 进程不提供原生桌面窗口，壳是应用的唯一入口 |
| 后端 | `dsh-server`（SEA，内嵌 node 的 `dsh --profile web`） | 跑 dsh 的 cordis 插件树，HTTP 伺服前端与 API | dsh 是 node/TypeScript 的 cordis 插件化 harness，只能在 node 上运行，Go 无法替代 |
| 前端 | dsh 内置 web 前端（`@deepseek-ai/dsh-web-app`） | 浏览器 UI，由后端经 HTTP 伺服，WebView 加载 | UI 是浏览器应用，且后端经 HTTP 注入 `__DSH_BOOT__` 引导，无法脱离后端直接打开 |

三个关键点：

- **必须依赖 node 运行时**：cordis 插件树（TS/ESM/npm 生态）只能在 node 上
  跑，桌面 app 不假设用户装了 node，因此用 SEA（Node Single Executable
  Application）把 node 内嵌进单文件可执行。构建期 node 由 mise 提供。
- **必须走 HTTP**：`dsh --profile web` 以 HTTP 伺服前端与 API
  （`__DSH_BOOT__` 注入），壳的 WebView 加载 `http://127.0.0.1:<port>`，
  端口由 OS 分配避免冲突。
- **壳必须存在**：node 后端不提供原生窗口；壳承担窗口生命周期、后端守护，
  并在退出时终止后端进程组，不留孤儿 node。

## 构建原理

`bundle <workspace>` 依次执行：

1. **依赖闭包**（`internal/profile`）：工作区工程文件缺失时从内嵌模板兜底
   生成（pnpm-workspace.yaml 的 `nodeLinker: hoisted` 保证扁平闭包、
   `autoInstallPeers` 保证 dsh 核心 peer 依赖、`allowBuilds` 放行原生模块
   构建脚本）；未安装时在工作区 `pnpm install`（与 dsh 官方一致，复用已有
   安装）。安装器定位真实 pnpm（`internal/pm`：`mise which pnpm` 优先、
   拒绝 nub shim、缺失明确报错）。
2. **SEA 打包**（`internal/sea`）：工作区 node_modules 闭包解引用复制到
   `target/<name>/sea/node_modules`，复制 `@deepseek-ai/dsh` 的 `config/`
   与 `package.json`，生成 `sea-entry.mjs`（dsh CLI 入口）与
   `tsdown.config.mjs`，调用工具链 tsdown（`target/tools`，pnpm 安装）产出
   `sea/bin/dsh`。bundle 插件包（含依赖闭包）全部一并打包。
3. **壳构建**：`go build .`（Wails v3；壳源码由模块根 package main
   （`internal/shell/`）与 `server/` 包组成）。构建输入（壳源码与精简
   go.mod）由 `internal/cli/shellsrc` 以 go:embed 内嵌在 CLI 二进制中，
   运行时解出到 `target/<name>/.shell-src/`（壳专用模块根）再构建——
   CLI 不依赖仓库 checkout，`go install` 后也能 bundle（脱离源码树）。
   副本由 `scripts/sync-shellsrc.sh`（`just sync-shell-src`，goreleaser
   发布前 before hook 执行同一脚本）从仓库同步，并在 `_src` 内
   `go mod tidy` 把 go.mod 精简到只含壳依赖（去掉 tool 指令与 CLI 专用
   依赖）；改壳源码或依赖后必须重新同步，`go test` 会校验副本一致性。
4. **平台组装**（`internal/bundle`）：macOS `.app`（Info.plist、icns）、
   Linux 目录 + `tar.gz`（hicolor 图标集）、Windows 目录 + `zip`（ico），
   写入壳同目录 `appconfig.json` 与 DSH_HOME 种子 `dsh-home/`
   （`profiles/web` ← 工作区解引用复制；`settings.yaml`、`storages/`、
   `sessions/` 等用户运行时数据不进种子）。

图标渲染不依赖外部工具：SVG 源用 oksvg/rasterx（纯 Go）渲染为白底图，
PNG 源直接解码，各平台尺寸由 `golang.org/x/image/draw` 缩放；macOS icns
用系统 iconutil 打包。

## 环境变量

- 构建期：`DSH_DESKTOP_ROOT`（仓库根；`go install` 到 PATH 后使用，
  仓库内 `go tool` 时自动定位）。壳构建不依赖它（源码由 CLI 内嵌），
  仅用于把 `examples/<name>` 相对路径解析到仓库内工作区
- 运行时（壳）：`DSH_APP_DSH_HOME`（显式覆盖 DSH_HOME，开发/测试用）、
  `DSH_APP_WORKSPACE`（工作目录，默认用户主目录）、`DSH_APP_PORT`
  （后端端口，默认 `0` 由 OS 分配）
- 透传给后端：`DEEPSEEK_API_KEY` / `DEEPSEEK_BASE_URL`（LLM 凭据，Unix
  上启动前按 `$SHELL` source 用户 shell 配置继承）
