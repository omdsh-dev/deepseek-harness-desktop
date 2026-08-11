export OUTPUT_DIR := join(justfile_directory(), "target")

# deepseek-harness-desktop 构建入口。
#
# 用法（仓库根执行）：
#   just sea                构建 SEA 单文件可执行（含外置资源）
#   just build-macos-app    完整构建 macOS 应用（SEA + Wails 壳 + 图标 + 组装）
#   just install-macos-app  构建并安装到系统应用目录 /Applications
#   just run-macos-app      运行 macOS 构建产物
#   just build-linux-app    完整构建 Linux 应用（SEA + Wails 壳 + 组装，须在 Linux 上执行）
#   just build-windows-app  完整构建 Windows 应用（SEA + Wails 壳 + 组装，须在 Windows 上执行）
#
# 产物统一在仓库根 target/ 下：target/sea、target/DSH.app、target/dsh.icns、
# target/dsh.iconset/（macOS 多尺寸图标集）、target/linux/、target/windows/。
#
# [working-directory] 相对「本 justfile 所在目录（仓库根）」解析：
# `deepseek-harness` 指向 deepseek-harness，`.` 即仓库根。

default:
    just --list

dep:
    nub install

sea:
    nubx tsdown -c tsdown.sea.config.ts
    nubx zx scripts/sea-materialize.mts
    @echo "SEA 产物: target/sea/bin/dsh"

# 从 apps/dsh-desktop/assets/icon.svg 生成 app 图标 target/dsh.icns
# （见 scripts/icon.mts：resvg 渲染 + iconutil 打包）。
icon:
    nubx zx scripts/icon.mts

make-macos-app: icon
    nubx zx scripts/make-macos-app.mts

# 组装桌面应用 target/DSH.app：拷贝 SEA 资源与图标，并直接把 Wails 壳
# （go build，普通 Go 程序，无需中间产物）输出到 Contents/MacOS/dsh-shell。
# 前置：sea + icon。
[working-directory("apps/dsh-desktop")]
bundle-macos-app: make-macos-app
    go build -o {{ OUTPUT_DIR }}/DSH.app/Contents/MacOS/dsh-shell .
    chmod 755 {{ OUTPUT_DIR }}/DSH.app/Contents/MacOS/dsh-shell

# 完整构建：SEA + 图标 + 组装（含壳）。
build-macos-app: sea bundle-macos-app

# 安装到系统应用目录 /Applications（先删旧版再拷贝，确保全新）。
install-macos-app: build-macos-app
    rm -rf /Applications/DSH.app
    cp -R {{ OUTPUT_DIR }}/DSH.app /Applications/DSH.app
    @echo "已安装: /Applications/DSH.app"

# 运行构建产物。
run-macos-app:
    open target/DSH.app

# 组装 Linux 桌面应用 target/linux/DSH/：拷贝 SEA 资源与图标 png，打包
# DSH.tar.gz（见 scripts/make-linux-app.mts）。
make-linux-app:
    nubx zx scripts/make-linux-app.mts

# 组装 Linux 桌面应用 target/linux/DSH/：拷贝 SEA 资源与图标 png，壳由
# go build 输出到 bin/dsh-shell，并打包 DSH.tar.gz（见 scripts/make-linux-app.mts）。
# 仅在 Linux 上执行：SEA（Node --build-sea）与 Wails 壳（cgo WebKitGTK）
# 均不支持交叉编译，产物必须由 Linux 主机产出（macOS 的 target/sea 不适用）。
# 前置：sea。
[working-directory("apps/dsh-desktop")]
bundle-linux-app: make-linux-app
    go build -o {{ OUTPUT_DIR }}/linux/DSH/bin/dsh-shell .
    chmod 755 {{ OUTPUT_DIR }}/linux/DSH/bin/dsh-shell

# 完整构建：Linux 版 SEA + 组装（含壳）。
build-linux-app: sea bundle-linux-app

# 组装 Windows 桌面应用 target/windows/DSH/：拷贝 SEA 资源与图标，打包
# DSH.zip（见 scripts/make-windows-app.mts）。
make-windows-app:
    nubx zx scripts/make-windows-app.mts

# 组装 Windows 桌面应用 target/windows/DSH/：拷贝 SEA 资源与图标，壳由
# go build 输出到 bin/dsh-shell.exe，并打包 DSH.zip（见 scripts/make-windows-app.mts）。
# 仅在 Windows 上执行：SEA（Node --build-sea）与 Wails 壳（WebView2）均不支持
# 交叉编译，产物必须由 Windows 主机构建（macOS 的 target/sea 不适用）。
# -ldflags "-H=windowsgui" 让壳以 GUI 子系统运行，启动时不弹控制台黑窗。
[working-directory("apps/dsh-desktop")]
bundle-windows-app: make-windows-app
    go build -ldflags "-H=windowsgui" -o {{ OUTPUT_DIR }}/windows/DSH/bin/dsh-shell.exe .

# 完整构建：Windows 版 SEA + 组装（含壳）。
build-windows-app: sea bundle-windows-app

clean:
    rm -f nub.lock;
    rm -rf node_modules;

dsh *args:
    node \
    --expose-internals \
    --import tsx/esm \
    ./node_modules/@deepseek-ai/dsh/lib/bin.js {{ args }}
