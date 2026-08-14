# 工作区（workspace）

工作区是**拍平的 desktop 定义**：desktop 有且只有一个 profile（web），
profile 内容直接放在工作区根（无目录嵌套、无独立配置文件）：

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

示例（[examples/official](../examples/official)）：

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

## dsh.desktop 字段

- `id` — bundle 标识（macOS CFBundleIdentifier；缺省由 name 派生）
- `window` — 窗口几何（缺省 1280x800，最小 800x600）
- `icon` — 相对工作区的图标源（SVG 或 PNG）
- `dshHome` — 运行时 DSH_HOME 策略：
  - 缺省 / `xdg` — `xdg.DataHome/<name>`（[adrg/xdg](https://github.com/adrg/xdg)
    规范：Linux `~/.local/share`、macOS `~/Library/Application Support`
    等），与 `dev` 的运行时 home 一致。应用内置 dsh-home 种子，首次启动
    把缺失部分拷贝进该目录，之后读写都在拷贝上，完全独立、不污染 `~/.dsh`
  - `env` — 不设置 DSH_HOME，继承环境（`$DSH_HOME` 或默认 `~/.dsh`）
  - 绝对路径 — DSH_HOME 固定为该路径，缺失部分从应用种子补齐

## 先验证，再打包

工作区本身就是可安装、可验证的单元，用官方 dsh 流程：

```sh
cd examples/custom
pnpm install                                    # 依赖闭包落在工作区 node_modules
./node_modules/.bin/dsh plugin --profile web add @morlay/session-persistence-rdb  # 官方装 bundle
DSH_HOME=$XDG_DATA_HOME/dsh ./node_modules/.bin/dsh web --patch ./cordis.patch.yml  # 官方跑 web + 工作区 patch
```

加插件也可以直接用本命令的 `plugin add`（等价于上面的 dsh plugin 流程，
但目标固定为工作区，不碰全局 `~/.dsh`）：

```sh
cd examples/custom
deepseek-harness-desktop plugin add @morlay/session-persistence-rdb   # 工作区 pnpm add + bundles reconcile
deepseek-harness-desktop plugin add --workspace examples/custom @foo/bar   # 从任意目录指定工作区
```

`plugin add` 复用 dev 的 DSH_HOME 布局（`xdg.DataHome/<name>/profiles/web`
→ 工作区），调用工作区闭包里的 `dsh plugin --profile web add`：在工作区跑
`pnpm add`，成功后在 `package.json` 里 reconcile `dsh.profile.bundles`——
依赖中声明 `dsh.bundle`（patch 层）的包自动入层，被移除/失去声明的包出层，
与官方 dsh 语义完全一致。

patch 与插件组合确认可用后，`bundle` 只是把它包装为桌面应用（复用工作区
已安装的闭包，不再重复安装）。`dev` 命令做的也是同一件事：DSH_HOME 固定为
`xdg.DataHome/<name>`，`profiles/web` 符号链接指向工作区，再起 `dsh web`
并打开浏览器。

## patch 合成语义

dsh 的 cordis 配置是分层 patch 合成：`dsh.profile.bundles` 按序叠加各
bundle 包自带的 patch 层，最后叠加 `cordis.patch.yml`（用户层）。CLI 只
负责安装、打包与分发，不修改任何 patch 语义。`settings.yaml`、`storages/`、
`sessions/` 等用户运行时数据不属于工作区，首次启动后由应用在目标 DSH_HOME
中生成。打包时工作区被装配为应用的 DSH_HOME 种子（dsh 固定从
`$DSH_HOME/profiles/web` 解析 profile）。
