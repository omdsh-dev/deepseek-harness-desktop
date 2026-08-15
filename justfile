# deepseek-harness-desktop —— 单命令（纯 Go）便捷入口。
# 命令已通过 go.mod 的 tool 指令注册：go tool deepseek-harness-desktop。

# 打包指定工作区（默认本机平台）。
#   just bundle examples/custom
bundle *args: sync-shell-src
    go tool deepseek-harness-desktop bundle {{ args }}

# 把壳源码（internal/shell 与 server/）与精简 go.mod 同步到 CLI 的 embed
# 副本（internal/cli/shellsrc/_src，见 scripts/sync-shellsrc.sh）。
# 构建/发布 CLI 前必须执行：CLI 二进制在运行时用这份副本解出并 go build 壳，
# 脱离源码树（go install 后 bundle）。改了壳源码后先跑这个。
sync-shell-src:
    ./scripts/sync-shellsrc.sh

# 开发模式：基于工作区起 dsh web 并打开浏览器。
#   just dev examples/custom
dev *args:
    go tool deepseek-harness-desktop dev {{ args }}

# 向工作区添加 dsh 插件（代理 dsh plugin add，修改工作区 bundles）。
#   just plugin add --workspace examples/custom @foo/bar
#   cd examples/custom && just plugin add @foo/bar
plugin *args:
    go tool deepseek-harness-desktop plugin {{ args }}

test:
    go test ./...

clean:
    rm -rf target

install:
    go install ./cmd/deepseek-harness-desktop