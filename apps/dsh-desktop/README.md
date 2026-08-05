# dsh-desktop

把 dsh 封装成不依赖外部浏览器的 macOS 桌面应用（Wails v3 壳）。

## 架构

```
DSH.app/Contents/
  MacOS/dsh-shell    Wails 壳（本 Go 程序，CFBundleExecutable）
  MacOS/dsh-server   SEA 可执行（内嵌 node v26.5.0 的 `dsh web`）
  config/ node_modules/ package.json   运行时资源（dsh-server 从上一级解析）
```

壳是唯一入口，同时是 SEA 后端的守护进程：启动同目录
`dsh-server web --port 0`（端口由 OS 分配，避免冲突），从后端 stdout 解析
实际监听地址，用 WebviewWindow 内嵌加载 —— 全程不打开系统浏览器。
后端异常退出（网络/加载失败等）时自动退避重启（1s 起、上限 30s）并重新
指向新地址。应用退出（窗口关闭 / Cmd+Q / 外部 SIGTERM）时终止后端：
SIGTERM 其进程组（含 SEA 内嵌 node 的 dsh-server，及将来可能 spawn 的子
进程），宽限期内未退出则 SIGKILL 兜底，确保不留孤儿 node；main 会等守护
协程收口后才真正退出。壳进程注册了 SIGTERM/SIGINT 处理：外部 kill 不会
直接终止壳（那样清理逻辑没机会执行），而是走同一收口路径。
窗口关闭（点关闭按钮）也走同一收口：直接监听 Wails 的关闭事件主动
quit()，不依赖 NSApplication 的 shouldTerminateAfterLastWindowClosed
委托链路（该链路在部分环境不生效，窗口关了但事件循环不退）。
后端的 stdout 管道在解析出 URL 后仍持续排空到 EOF——防止后端写日志时
管道缓冲填满而阻塞在 write，拖住优雅退出。

为什么要三层（壳 + 后端 + Web 前端）：dsh 是跑在 node 上的 cordis 插件化
harness（插件运行时按包名 `import()`、npm 依赖闭包），无法移植到 Go，所以
后端必须是 node 进程；`dsh web` 又以 HTTP 伺服前端与 API（含 `__DSH_BOOT__`
注入），WebView 只能经 HTTP 加载，前端因此是独立一层。node 进程不提供原生
窗口，也不假设用户装了 node——壳用 Wails 提供窗口与守护，后端用 SEA
（内嵌 node v26.5.0）单文件分发。详见根 README「架构」小节。

## 代码结构

```text
main.go       应用装配：解析环境变量（DSH_APP_WORKSPACE / DSH_APP_PORT）、
              创建窗口、信号处理、退出收口（等守护协程终止后端进程组）
supervise.go  守护循环：启动后端 → 就绪后 SetURL 指向实际地址 → 异常退出
              时退避重启（1s 起、上限 30s），应用退出时收口
server/       子包：SEA 后端进程生命周期（Start / Process.Stop / Exit、
              启动命令构造、URL 就绪行解析），不依赖 Wails，可独立测试
```

## 构建

构建入口在仓库根 `justfile`；构建脚本在根 `scripts/` 下
（TypeScript + [zx](https://google.github.io/zx/)）。从仓库根执行：

```sh
just sea                 # 构建 SEA 单文件可执行（含外置资源）
just build-macos-app     # 完整构建（SEA + Wails 壳 + 组装 target/DSH.app）
just install-macos-app   # 构建并安装到 /Applications/DSH.app
just run-macos-app       # 运行 target/DSH.app
open target/DSH.app      # 或直接打开构建产物
just build-linux-app     # Linux 完整构建（须在 Linux 主机上执行，见根 README）
```

启动页面：窗口先显示内嵌的"正在启动 dsh…"HTML（非 Wails 默认空白页），
后端就绪后自动切到真实地址。

## 环境变量

壳（加载前读取，窗口/后端启动前生效）：
- `DSH_APP_WORKSPACE` — 工作目录（默认用户主目录；受限/测试环境可覆盖）
- `DSH_APP_PORT` — 后端监听端口（默认 `0` 由 OS 分配随机端口，避免冲突；
  显式指定则固定复用该端口）

后端（dsh-server）继承的环境变量：启动前按 `$SHELL` source 用户 shell 配置
（bash → `~/.bashrc`，zsh → `~/.zshrc`），使后端继承用户终端里 export 的
变量；source 输出重定向到 /dev/null，不污染后端 stdout；用 `exec` 保持
同一进程（PID 不变），守护 wait 语义不受影响。常用透传变量：
- `DSH_HOME` — 个人配置目录：`$DSH_HOME/config.yaml` 配置覆盖、`$DSH_HOME/.env` 凭据
- `DEEPSEEK_API_KEY` / `DEEPSEEK_BASE_URL` — LLM 凭据（也可放调用目录 `.env`）
