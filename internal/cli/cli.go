// Package cli 实现 deepseek-harness-desktop 单命令：把工作区（examples/
// 下的拍平 desktop 定义：package.json + cordis.patch.yml + dsh.desktop.
// yaml）打包为独立自定义桌面。
//
// 用法（go install 后任意目录，或仓库内 go tool）：
//
//	deepseek-harness-desktop dev <workspace>                 开发模式：构建并直接运行
//	deepseek-harness-desktop bundle --platform=os/arch <ws>  打包平台应用（默认本机平台）
//
// 选项：
//
//	--skip-install   跳过依赖安装（使用已有安装）
//
// 全部产物在仓库根 target/ 下（target/tools 工具链、target/<name>/ 各
// desktop 的 profile 安装 / SEA / 应用包）。
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/omdsh-dev/deepseek-harness-desktop/internal/bundle"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/config"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/sea"
)

const usage = `deepseek-harness-desktop — 把 dsh 的 --profile web 与 cordis.patch.yml 打包为独立自定义桌面。

用法（go install 后任意目录，或仓库内 go tool）：
  deepseek-harness-desktop dev <workspace>                  开发模式：构建并直接运行
  deepseek-harness-desktop bundle --platform=os/arch <ws>   打包平台应用（默认本机平台）

选项：
  --skip-install   跳过依赖安装（使用已有安装）

工作区是拍平的 desktop 定义（见 examples/official、examples/custom）：
  package.json       全部配置：name/version/dependencies（npm 语义）、
                     dsh.profile.bundles、dsh.desktop（id/window/icon/dshHome）
  cordis.patch.yml   profile patch 层（dsh 应用在 bundle 层之后）
  icon.svg           应用图标（可选，dsh.desktop.icon 引用）

settings.yaml 等用户运行时数据不属于工作区：首次启动后由应用在
DSH_HOME（XDG_DATA_HOME/<name>/dsh-home）中生成。

全部产物在仓库根 target/ 下。
`

// Run 执行 CLI，返回进程退出码。
func Run(args []string) int {
	skipInstall := false
	platform := ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--skip-install":
			skipInstall = true
		case a == "--platform":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--platform 需要参数（os/arch，如 macos/arm64）")
				return 2
			}
			i++
			platform = args[i]
		case strings.HasPrefix(a, "--platform="):
			platform = strings.TrimPrefix(a, "--platform=")
		default:
			rest = append(rest, a)
		}
	}

	if len(rest) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	switch rest[0] {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return 0
	case "dev":
		if len(rest) < 2 {
			fmt.Fprintln(os.Stderr, "用法：deepseek-harness-desktop dev <workspace>")
			return 2
		}
		if err := Dev(rest[1], skipInstall); err != nil {
			fmt.Fprintf(os.Stderr, "dev 失败：%v\n", err)
			return 1
		}
		return 0
	case "bundle":
		if len(rest) < 2 {
			fmt.Fprintln(os.Stderr, "用法：deepseek-harness-desktop bundle [--platform=os/arch] <workspace>")
			return 2
		}
		if _, err := Bundle(rest[1], platform, skipInstall); err != nil {
			fmt.Fprintf(os.Stderr, "bundle 失败：%v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知命令 %q\n\n%s", rest[0], usage)
		return 2
	}
}

// checkPlatform 校验 bundle 目标平台（SEA 与 Wails 壳均不支持交叉编译）。
func checkPlatform(platform string) error {
	if platform == "" {
		return nil
	}
	parts := strings.SplitN(platform, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("--platform 必须是 os/arch 形式（如 macos/arm64），得到 %q", platform)
	}
	if parts[0] != runtime.GOOS || parts[1] != runtime.GOARCH {
		return fmt.Errorf("不支持交叉编译：SEA（node --build-sea）与 Wails 壳只能在本机平台构建（当前 %s/%s，目标 %s）", runtime.GOOS, runtime.GOARCH, platform)
	}
	return nil
}

// Bundle 执行一次完整打包（profile 安装 → SEA → 壳 → 平台组装），返回
// 产物路径。
func Bundle(ws, platform string, skipInstall bool) (string, error) {
	if err := checkPlatform(platform); err != nil {
		return "", err
	}
	root, ws, cfg, err := loadWorkspace(ws)
	if err != nil {
		return "", err
	}

	fmt.Printf("==> 打包 %s（%s %s）\n", cfg.Name, config.ProfileName, cfg.Version)

	// 1) SEA 后端。
	seaExe, err := sea.Build(root, ws, cfg, skipInstall)
	if err != nil {
		return "", err
	}
	fmt.Printf("==> SEA 后端: %s\n", seaExe)

	// 2) 壳二进制。
	shellBin, err := buildShell(root, ws, cfg)
	if err != nil {
		return "", err
	}

	// 3) 平台组装。
	appRoot, err := bundle.Assemble(bundle.Inputs{
		Root:      root,
		Workspace: ws,
		Cfg:       cfg,
		SeaExe:    seaExe,
		ShellBin:  shellBin,
	})
	if err != nil {
		return "", err
	}
	fmt.Printf("==> 产物: %s\n", appRoot)
	return appRoot, nil
}

// Dev 构建并直接运行（开发布局 target/<name>/dev，不组装平台应用）。
func Dev(ws string, skipInstall bool) error {
	root, ws, cfg, err := loadWorkspace(ws)
	if err != nil {
		return err
	}

	fmt.Printf("==> dev %s（%s %s）\n", cfg.Name, config.ProfileName, cfg.Version)

	// 1) SEA 后端。
	seaExe, err := sea.Build(root, ws, cfg, skipInstall)
	if err != nil {
		return err
	}

	// 2) 壳二进制。
	shellBin, err := buildShell(root, ws, cfg)
	if err != nil {
		return err
	}

	// 3) 开发布局（target/<name>/dev）。
	binDir, err := bundle.AssembleDev(bundle.Inputs{
		Root:      root,
		Workspace: ws,
		Cfg:       cfg,
		SeaExe:    seaExe,
		ShellBin:  shellBin,
	})
	if err != nil {
		return err
	}

	// 4) 构造运行时 DSH_HOME（target/<name>/dsh-home）：dsh 固定从
	//    $DSH_HOME/profiles/web 解析 profile，profiles/web 用符号链接指向
	//    工作区——用户在工作区的 pnpm install 结果直接可见，无需复制。
	homeDir := config.DSHHomeDir(root, cfg)
	if err := os.MkdirAll(filepath.Join(homeDir, "profiles"), 0o755); err != nil {
		return err
	}
	profileLink := filepath.Join(homeDir, "profiles", config.ProfileName)
	if _, err := os.Lstat(profileLink); os.IsNotExist(err) {
		if err := os.Symlink(ws, profileLink); err != nil {
			return fmt.Errorf("构造 profiles/web 链接: %w", err)
		}
	}

	// 5) 启动。
	shellName, _ := bundle.BinNames()
	shell := filepath.Join(binDir, shellName)
	fmt.Printf("==> 启动 %s（DSH_HOME=%s）\n", shell, homeDir)
	return runDetachedEnv(shell, []string{"DSH_APP_DSH_HOME=" + homeDir})
}

// loadWorkspace 解析工作区并返回（仓库根, 绝对工作区路径, 配置）。
// `examples/<name>` 形式始终解析到仓库根的 examples/ 目录；其余路径按
// 当前目录解析。
func loadWorkspace(ws string) (string, string, *config.Config, error) {
	root, err := repoRoot()
	if err != nil {
		return "", "", nil, err
	}
	if !filepath.IsAbs(ws) {
		if ws == "examples" || strings.HasPrefix(ws, "examples"+string(filepath.Separator)) {
			ws = filepath.Join(root, ws)
		}
	}
	ws, err = filepath.Abs(ws)
	if err != nil {
		return "", "", nil, err
	}
	cfg, err := config.Load(ws)
	if err != nil {
		return "", "", nil, err
	}
	return root, ws, cfg, nil
}

// repoRoot 返回仓库根（internal/cli 源文件上三级；go run / 源码树构建
// 均有效）。DSH_DESKTOP_ROOT 环境变量可显式覆盖（go install 后使用）。
func repoRoot() (string, error) {
	if v := os.Getenv("DSH_DESKTOP_ROOT"); v != "" {
		return v, nil
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("无法定位仓库根；设置 DSH_DESKTOP_ROOT 或从仓库根运行")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file))), nil
}
