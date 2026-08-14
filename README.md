# deepseek-harness-desktop

把 [@deepseek-ai/dsh](https://www.npmjs.com/package/@deepseek-ai/dsh) 的
`--profile web` 与 `cordis.patch.yml` 打包为**独立自定义桌面**的单命令仓库。
仓库根是纯 Go：唯一的命令 `deepseek-harness-desktop` 接收一个工作区
（examples/ 下的拍平 desktop 定义），完成 profile 依赖安装、SEA 后端
打包、Wails 壳构建与平台组装。全部产物在仓库根 `target/` 下。支持
macOS、Linux 与 Windows。

## 快速开始

依赖（[mise](https://mise.jdx.dev) 管理）：`go`、`nub`，见 [mise.toml](./mise.toml)。
dsh 本体通过 npm 安装到构建目录（`nub install`），根仓库不持有任何 npm
清单。

```sh
go tool deepseek-harness-desktop bundle examples/official     # 打包官方 web profile
go tool deepseek-harness-desktop bundle examples/custom       # 打包自定义工作区
go tool deepseek-harness-desktop dev examples/official        # 开发模式：构建并直接运行
go tool deepseek-harness-desktop ls                           # 列出 examples 工作区
```

命令已通过 go.mod 的 `tool` 指令注册（`go 1.24+`）：`go tool
deepseek-harness-desktop` 自动构建并运行，无需手动 `go build`。也可以
`go install github.com/omdsh-dev/deepseek-harness-desktop/cmd/deepseek-harness-desktop@latest`
安装到 PATH 后直接使用命令名（仓库根定位见「环境变量」）。

macOS 产物：`target/<name>/<Name>.app`；Linux：`target/<name>/linux/<Name>/`
（含 `tar.gz`）；Windows：`target/<name>/windows/<Name>/`（含 `zip`）。
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

## 工作区（examples/）

工作区是一个**拍平的 desktop 定义**（无独立配置文件、无目录嵌套），
desktop 有且只有一个 profile（web）：

```text
examples/custom/
  package.json        全部配置：
                      - name/version/dependencies（npm 语义，直接复用）
                      - dsh.profile.bundles：cordis bundle 列表
                      - dsh.desktop：桌面特有（id/window/icon/dshHome）
  cordis.patch.yml    profile patch 层（dsh 应用在 bundle 层之后）
  icon.svg            应用图标（可选，dsh.desktop.icon 引用）
```

dsh 的 cordis 配置是分层 patch 合成：`dsh.profile.bundles` 按序叠加各
bundle 包自带的 patch 层，最后叠加 `cordis.patch.yml`（用户层）。CLI 只
负责把这份布局安装（`nub install`）、打包（SEA + 壳 + 组装）与分发，不
修改任何 patch 语义。`settings.yaml`、`storages/`、`sessions/` 等用户
运行时数据不属于工作区：首次启动后由应用在目标 DSH_HOME 中生成。

`package.json` 示例（examples/official）：

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
- `icon` — 相对工作区的图标源（SVG）
- `dshHome` — 运行时 DSH_HOME 策略：
  - 缺省 / `xdg` — `xdg.DataHome/<name>/dsh-home`（[adrg/xdg](https://github.com/adrg/xdg)
    规范：Linux `~/.local/share`、macOS `~/Library/Application Support`
    等）。bundle 内置 dsh-home 种子，首次启动拷贝到该目录，之后读写都在
    拷贝上，完全独立、不污染 `~/.dsh`
  - `env` — 不设置 DSH_HOME，继承环境（`$DSH_HOME` 或默认 `~/.dsh`）
  - 绝对路径 — DSH_HOME 固定为该路径，缺失部分从 bundle 种子补齐

## 构建原理（单命令做了什么）

`bundle <workspace>` 依次执行：

1. **profile 装配与安装**：把工作区拍平内容装配为 DSH_HOME 布局
   `target/<name>/dsh-home/`（`profiles/web/package.json` 注入原生包信任
   `allowBuilds`、`profiles/web/cordis.patch.yml`、`pnpm-workspace.yaml`
   与 `.npmrc`——含 registry 映射 `@deepseek-ai` → npmjs、`@morlay` →
   GitHub npm，以及构建目录本地 store 覆盖），运行 `nub install` 安装依赖
   闭包（扁平布局，`autoInstallPeers` 保证 dsh 核心 peer 依赖完整）。
2. **SEA 打包**（`internal/sea`）：把 profile 的 node_modules 闭包解引用
   复制到 `target/<name>/sea/node_modules`，复制 `@deepseek-ai/dsh` 的
   `config/` 与 `package.json`，生成 `sea-entry.mjs`（dsh CLI 入口）与
   `tsdown.config.mjs`，调用工具链 tsdown（`target/tools`）产出
   `sea/bin/dsh`。bundle 插件包（含 `dsh.desktop` 之外的依赖闭包）全部
   一并打包进 SEA。
3. **壳构建**：`go build ./internal/shell`（Wails v3）。
4. **平台组装**（`internal/bundle`）：macOS `.app`（Info.plist、icns）、
   Linux 目录 + `tar.gz`（hicolor 图标集）、Windows 目录 + `zip`（ico），
   写入壳同目录 `appconfig.json` 与 DSH_HOME 种子 `dsh-home/`（不含用户
   运行时数据）。

`dev <workspace>` 组装开发布局 `target/<name>/dev/`（壳 + SEA + 资源 +
dsh-home）并直接启动，DSH_HOME 指向构建出的 `target/<name>/dsh-home`。

## 目录结构

- `cmd/deepseek-harness-desktop/` — 单命令入口（可 `go install`）
- `internal/cli/` — 命令分发（bundle / dev / ls）
- `internal/config/` — 工作区配置解析（package.json 的
  `dsh.profile.bundles` 与 `dsh.desktop`）
- `internal/profile/` — 拍平装配为 DSH_HOME 布局 + `nub install`
- `internal/tools/` — 构建工具链（`target/tools`：tsdown / sharp）
- `internal/sea/` — SEA 打包（staging + 生成入口/配置 + tsdown）
- `internal/shell/` — Wails 壳（窗口 + 后端守护 + XDG DSH_HOME 策略），
  构建为独立二进制 `dsh-shell`；`internal/shell/server/` 是后端进程生命周期
  （平台差异：Unix 进程组信号 / Windows Job Object）
- `internal/bundle/` — 平台组装、dev 布局与图标生成
- `internal/fsutil/` — 构建/运行时共用复制工具
- `examples/official/` — 官方 web profile 打包用例（无自定义）
- `examples/custom/` — 自定义桌面用例（附加
  `@morlay/session-persistence-rdb` 插件 bundle + patch 禁用 JSONL 持久化）

## 环境变量

- 构建期：`DSH_DESKTOP_ROOT`（仓库根；`go install` 到 PATH 后需要，
  `go run` 从仓库根执行时自动定位）
- 运行时（壳）：`DSH_APP_DSH_HOME`（显式覆盖 DSH_HOME，开发/测试用）、
  `DSH_APP_WORKSPACE`（工作目录，默认用户主目录）、`DSH_APP_PORT`
  （后端端口，默认 `0` 由 OS 分配）
- 透传给后端：`DEEPSEEK_API_KEY` / `DEEPSEEK_BASE_URL`（LLM 凭据，Unix
  上启动前按 `$SHELL` source 用户 shell 配置继承）
