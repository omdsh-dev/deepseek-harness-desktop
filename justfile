# deepseek-harness-desktop —— 单命令（纯 Go）便捷入口。
# 命令已通过 go.mod 的 tool 指令注册：go tool deepseek-harness-desktop。

# 打包指定工作区（默认本机平台）。
#   just bundle examples/custom
bundle *args:
    go tool deepseek-harness-desktop bundle {{ args }}

# 开发模式：构建并直接运行。
#   just dev examples/custom
dev *args:
    go tool deepseek-harness-desktop dev {{ args }}

# 列出 examples 工作区。
ls:
    go tool deepseek-harness-desktop ls

test:
    go test ./...

clean:
    rm -rf target
