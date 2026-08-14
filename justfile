# deepseek-harness-desktop —— 单命令（纯 Go）便捷入口。
# 全部逻辑在 Go CLI（cmd/deepseek-harness-desktop），just 仅包装 go 命令。

# 打包指定工作区（默认本机平台）。
#   just bundle examples/custom
bundle *args:
    go run ./cmd/deepseek-harness-desktop bundle {{ args }}

# 开发模式：构建并直接运行。
#   just dev examples/custom
dev *args:
    go run ./cmd/deepseek-harness-desktop dev {{ args }}

# 列出 examples 工作区。
ls:
    go run ./cmd/deepseek-harness-desktop ls

# 构建 CLI 二进制到 target/。
cli:
    go build -o target/deepseek-harness-desktop ./cmd/deepseek-harness-desktop

test:
    go test ./...

clean:
    rm -rf target
