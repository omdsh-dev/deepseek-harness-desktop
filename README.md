# deepseek-harness-desktop

把 [@deepseek-ai/dsh](https://www.npmjs.com/package/@deepseek-ai/dsh) 的
`--profile web` 与 `cordis.patch.yml` 打包为**独立自定义桌面**的 Go 单命令。
支持 macOS、Linux 与 Windows。

## Quick Start

```sh
go install github.com/omdsh-dev/deepseek-harness-desktop/cmd/deepseek-harness-desktop@latest
```

在任意目录创建你的工作区（复制本仓库 [examples/custom](examples/custom)
即可起步），然后：

```sh
deepseek-harness-desktop dev examples/custom        # 开发模式：构建并直接运行
deepseek-harness-desktop bundle examples/custom     # 打包当前平台的应用
deepseek-harness-desktop bundle --platform=macos/arm64 examples/custom   # 显式声明平台
```

- `dev <workspace>` — 基于工作区直接起 `dsh web` 并打开浏览器页面（等价
  官方流程 `DSH_HOME=<xdg.DataHome>/<name> dsh web`，profiles/web 指向工作区；
  Ctrl+C 退出）
- `bundle <workspace>` — 打包为平台应用，产物在 `target/<name>/` 下

命令以仓库根下的 `target/` 为产物目录（全部产物集中于此）。`examples/<name>`
形式解析到仓库根的 `examples/`；也可以传绝对路径或相对路径。在仓库内开发
时可用 `go tool deepseek-harness-desktop`（已通过 go.mod 的 `tool` 指令注册，
无需 `go build`）。

构建依赖：`go`、`node`、`pnpm`（见 [mise.toml](mise.toml)，mise 管理）。
dsh 本体通过 pnpm 安装到构建目录，与 dsh 官方一致；仓库本身不持有任何
npm 清单。

## 工作区（workspace）

工作区是一个**拍平的 desktop 定义**（无目录嵌套、无独立配置文件），
desktop 有且只有一个 profile（web）：

```text
examples/custom/
  package.json        全部配置：
                      - name/version/dependencies（npm 语义，直接复用）
                      - dsh.profile.bundles：cordis bundle 列表
                      - dsh.desktop：桌面特有（id/window/icon/dshHome）
  cordis.patch.yml    profile patch 层（dsh 应用在 bundle 层之后）
  pnpm-workspace.yaml 安装工程文件（nodeLinker hoisted + allowBuilds）
  .npmrc              registry 映射（@morlay → GitHub npm）与本地 store
  icon.svg            应用图标（可选，dsh.desktop.icon 引用）
```

### 先验证，再打包

工作区本身就是可安装、可验证的单元，用官方 dsh 流程：

```sh
cd examples/custom
pnpm install                                    # 依赖闭包落在工作区 node_modules
./node_modules/.bin/dsh plugin --profile web add @morlay/session-persistence-rdb  # 官方装 bundle
DSH_HOME=$XDG_DATA_HOME/dsh ./node_modules/.bin/dsh web --patch ./cordis.patch.yml  # 官方跑 web + 工作区 patch
```

patch 与插件组合确认可用后，`bundle` 只是把它包装为桌面应用（复用工作区已
安装的闭包，不再重复安装）。

示例（[examples/official](examples/official)）：

```json
{
  "name": "dsh",
  "version": "0.1.0",
  "private": true,
  "dependencies": { "@deepseek-ai/dsh": "0.1.0-rc.6" },
  "dsh": {
    "profile": { "bundles": ["@deepseek-ai/dsh-base", "@deepseek-ai/dsh-web-app"] },
    "desktop": {
      "id": "ai.deepseek.dsh",
      "window": { "width": 1280, "height": 800, "minWidth": 800, "minHeight": 600 },
      "icon": "icon.svg",
      "dshHome": "xdg"
    }
  }
}
```

`dsh.desktop` 字段：

- `id` — bundle 标识（macOS CFBundleIdentifier；缺省由 name 派生）
- `window` — 窗口几何（缺省 1280x800，最小 800x600）
- `icon` — 相对工作区的图标源（SVG 或 PNG）
- `dshHome` — 运行时 DSH_HOME 策略：
  - 缺省 / `xdg` — `xdg.DataHome/<name>/dsh-home`（[adrg/xdg](https://github.com/adrg/xdg)
    规范：Linux `~/.local/share`、macOS `~/Library/Application Support`
    等）。应用内置 dsh-home 种子，首次启动拷贝到该目录，之后读写都在
    拷贝上，完全独立、不污染 `~/.dsh`
  - `env` — 不设置 DSH_HOME，继承环境（`$DSH_HOME` 或默认 `~/.dsh`）
  - 绝对路径 — DSH_HOME 固定为该路径，缺失部分从应用种子补齐

dsh 的 cordis 配置是分层 patch 合成：`dsh.profile.bundles` 按序叠加各
bundle 包自带的 patch 层，最后叠加 `cordis.patch.yml`（用户层）。CLI 只
负责安装、打包与分发，不修改任何 patch 语义。`settings.yaml`、`storages/`、
`sessions/` 等用户运行时数据不属于工作区，首次启动后由应用在目标 DSH_HOME
中生成。打包时工作区被装配为应用的 DSH_HOME 种子（dsh 固定从
`$DSH_HOME/profiles/web` 解析 profile）。

## 产物与架构

`bundle` 依次完成：pnpm 安装依赖闭包（扁平布局、`autoInstallPeers` 保证
dsh 核心 peer 依赖完整）→ SEA 打包（内嵌 node 的 `dsh --profile web`
单文件后端）→ Wails 壳构建 → 平台组装。产物：

| 平台 | 产物 |
|---|---|
| macOS | `target/<name>/<Name>.app`（Info.plist、icns） |
| Linux | `target/<name>/linux/<Name>/` + `tar.gz`（hicolor 图标集） |
| Windows | `target/<name>/windows/<Name>/` + `zip`（ico） |

桌面应用三层结构：**壳**（Wails v3，原生窗口 + WebView + 后端进程守护）
→ **后端**（SEA，内嵌 node 跑 dsh 的 cordis 插件树，HTTP 伺服前端与 API，
`__DSH_BOOT__` 注入）→ **前端**（dsh 内置 web UI）。壳从后端 stdout 解析
监听地址并加载 WebView，端口由 OS 分配；退出时终止后端进程组，不留孤儿
node。`bundle` 只支持本机平台（SEA 与 Wails 壳均不支持交叉编译），
`--platform=os/arch` 用于显式声明并校验。

## 环境变量

- 构建期：`DSH_DESKTOP_ROOT`（仓库根；`go install` 到 PATH 后使用，
  仓库内 `go tool` 时自动定位）
- 运行时（壳）：`DSH_APP_DSH_HOME`（显式覆盖 DSH_HOME，开发/测试用）、
  `DSH_APP_WORKSPACE`（工作目录，默认用户主目录）、`DSH_APP_PORT`
  （后端端口，默认 `0` 由 OS 分配）
- 透传给后端：`DEEPSEEK_API_KEY` / `DEEPSEEK_BASE_URL`（LLM 凭据，Unix
  上启动前按 `$SHELL` source 用户 shell 配置继承）
