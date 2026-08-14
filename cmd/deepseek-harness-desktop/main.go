// deepseek-harness-desktop —— 把 dsh 的 --profile web 与 cordis.patch.yml
// 打包为独立自定义桌面的单命令（CLI）。仓库根是纯 Go：工作区（examples/）
// 提供拍平的 desktop 定义（package.json + cordis.patch.yml + dsh.desktop.
// yaml），本命令完成 profile 安装、SEA 打包、壳构建与平台组装，并提供
// dev 开发模式。全部产物在仓库根 target/ 下。
package main

import (
	"os"

	"github.com/dsh-external/deepseek-harness-desktop/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
