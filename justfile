export OUTPUT_DIR := join(justfile_directory(), "target")

# deepseek-harness-desktop 构建入口。
#
# 用法（仓库根执行）：
#   just sea             构建 SEA 单文件可执行（含外置资源）
#   just build           完整构建（SEA + Wails 壳 + 图标 + 组装）
#   just install         构建并安装到系统应用目录 /Applications
#   just run             运行构建产物
#
# 产物统一在仓库根 target/ 下：target/sea、target/DSH.app、target/dsh.icns。
#
# [working-directory] 相对「本 justfile 所在目录（仓库根）」解析：
# `deepseek-harness` 指向 deepseek-harness，`.` 即仓库根。

default:
    just --list

sync:
    git clone --depth=1 {{ env("DEEPSEEK_HARNESS_REPO") }} ./deepseek-harness

dep:
    nub install

# 编译 deepseek-harness 的 lib 产物（sea 的前置；已有产物时由 just 增量跳过）。
# 对齐上游 `pnpm run build`（build:lib = tsc -b + tsdown；build:web = dsh-frontend
# 的 vite build，产出 apps/web/dist）。dsh web 启动时经
# require.resolve('@deepseek-ai/dsh-frontend/dist/index.html') 解析该产物，
# 缺失则后端启动即报 "frontend dist not built" 退出。vite 需在 apps/web 目录
# 运行（index.html 与 vite.config.ts 所在），故用 cd 切目录。
[working-directory("deepseek-harness")]
build-libs:
    OPENSSL_CONF=/dev/null nubx tsc -b && OPENSSL_CONF=/dev/null nubx tsdown && cd apps/web && OPENSSL_CONF=/dev/null nubx vite build

# 构建 SEA 单文件可执行：tsdown exe 内联 JS 依赖（--build-sea），
# 再实体化外置的运行时资源（config / package.json / 插件 node_modules）。
# 产物：target/sea/bin/dsh
sea: build-libs
    OPENSSL_CONF=/dev/null nubx tsdown -c tsdown.sea.config.ts
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
