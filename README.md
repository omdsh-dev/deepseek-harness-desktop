# deepseek-harness-desktop

为 [deepseek-harness](deepseek-harness/) 打包 macOS 桌面应用：Wails v3 壳
（`dsh-shell`）+ SEA 单文件后端（`dsh-server`，内嵌 node 的 `dsh web`），
Web 界面内嵌在 Webview 窗口里，不依赖外部浏览器。

## 快速开始

依赖（[mise](https://mise.jdx.dev) 管理）：`just`、`go`、`nub`，见 [mise.toml](./mise.toml)。

```sh
just sea                 # 构建 SEA 单文件可执行（含外置资源）
just build-macos-app     # 完整构建（SEA + Wails 壳 + 图标 + 组装 target/DSH.app）
just install-macos-app   # 构建并安装到 /Applications/DSH.app
just run-macos-app       # 运行 target/DSH.app
```

产物统一在 `target/`：`target/sea/`（SEA 产物与资源）、`target/DSH.app`（桌面应用）、`target/dsh.icns`（图标）。

## 目录结构

- `apps/dsh-desktop/` — Wails 壳（Go），桌面应用入口与后端守护，详见 [README](apps/dsh-desktop/README.md)
- `scripts/` — 构建脚本（TypeScript + [zx](https://google.github.io/zx/)）：
  `sea-materialize.mts`（SEA 运行时资源实体化）、`make-macos-app.mts`（DSH.app 组装）、`icon.mts`（图标生成）
- `deepseek-harness/` — 上游源代码（已 gitignore，**严禁修改**，见 [AGENTS.md](./AGENTS.md)）

## 构建说明

- `just sea` 依赖 `build-libs`：编译上游 lib（tsc + tsdown），并 `vite build` 产出前端
  `apps/web/dist`。`dsh web` 启动时经 `require.resolve('@deepseek-ai/dsh-frontend/dist/index.html')`
  解析该产物——缺失时后端启动即报 `dsh: frontend dist not built` 退出，表现为窗口停在启动页/空白。
- 构建命令统一带 `-macos-app` 后缀；`default` 列出全部 recipe（`just --list`）。
