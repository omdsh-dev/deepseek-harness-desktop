# AGENTS.md

## 开始工作前必读

- 技术栈见 [mise.toml](./mise.toml)
- 可用命令见 [justfile](./justfile)

这是 deepseek harness desktop 的仓库：以纯 Go 单命令
（cmd/deepseek-harness-desktop）把 @deepseek-ai/dsh 的 --profile web 与
cordis.patch.yml 打包为独立自定义桌面。工作区（examples/）只提供拍平的
desktop 定义（package.json + cordis.patch.yml），全部构建产物在 target/。
