// Package cli 实现 deepseek-harness-desktop 单命令：把工作区（examples/
// 下的拍平 desktop 定义：package.json + cordis.patch.yml）打包为独立
// 自定义桌面。
//
// 用法（go install 后任意目录，或仓库内 go tool）：
//
//	deepseek-harness-desktop dev <workspace>                  基于工作区起 dsh web 并打开浏览器
//	deepseek-harness-desktop bundle --platform=os/arch <ws>  打包平台应用（默认本机平台）
//	deepseek-harness-desktop plugin add <package...>         代理 dsh plugin add，修改工作区的 bundles
//
// 选项：
//
//	--skip-install   跳过依赖安装（使用已有安装）
//	--workspace=<ws> plugin 目标工作区（缺省当前目录）
//
// 全部产物在仓库根 target/ 下（target/tools 工具链、target/<name>/ 各
// desktop 的 profile 安装 / SEA / 应用包）。
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/adrg/xdg"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/bundle"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/config"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/fsutil"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/gitignore"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/profile"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/sea"
)

const usage = `deepseek-harness-desktop — 把 dsh 的 --profile web 与 cordis.patch.yml 打包为独立自定义桌面。

用法（go install 后任意目录，或仓库内 go tool）：
  deepseek-harness-desktop dev <workspace>                  基于工作区起 dsh web 并打开浏览器
  deepseek-harness-desktop bundle [--platform=os/arch] [--force] [--install] <workspace>
  deepseek-harness-desktop plugin add [--workspace=<path>] <package...>
                                                            代理 dsh plugin add：在工作区跑 pnpm add，
                                                            并把声明 dsh.bundle 的依赖加入
                                                            dsh.profile.bundles（不安装到全局 DSH_HOME）

选项：
  --platform=os/arch   声明目标平台（默认本机；SEA/壳不支持交叉编译）
  --force              忽略构建缓存，全新打包（默认基于工作区 dir hash 增量）
  --install            打包后安装到当前平台（macOS /Applications、
                       Linux XDG data + .desktop、Windows %LOCALAPPDATA%\Programs）
  --skip-install       跳过依赖安装（使用已有安装）
  --workspace=<path>   plugin add 的目标工作区（缺省当前目录）
  --profile=<name>     plugin add 兼容 dsh 写法；desktop 只有 web，仅接受 web

工作区是拍平的 desktop 定义（见 examples/official、examples/custom）：
  package.json       全部配置：name/version/dependencies（npm 语义）、
                     dsh.profile.bundles、dsh.desktop（id/window/icon/dshHome）
  cordis.patch.yml   profile patch 层（dsh 应用在 bundle 层之后）
  icon.svg           应用图标（可选，dsh.desktop.icon 引用）

settings.yaml 等用户运行时数据不属于工作区：首次启动后由应用在
DSH_HOME（XDG_DATA_HOME/<name>）中生成。

全部产物在仓库根 target/ 下。
`

// Run 执行 CLI，返回进程退出码。
func Run(args []string) int {
	skipInstall := false
	force := false
	install := false
	platform := ""
	workspace := ""
	profileName := ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--skip-install":
			skipInstall = true
		case a == "--force":
			force = true
		case a == "--install":
			install = true
		case a == "--platform":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--platform 需要参数（os/arch，如 macos/arm64）")
				return 2
			}
			i++
			platform = args[i]
		case strings.HasPrefix(a, "--platform="):
			platform = strings.TrimPrefix(a, "--platform=")
		case a == "--workspace":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--workspace 需要参数（工作区路径）")
				return 2
			}
			i++
			workspace = args[i]
		case strings.HasPrefix(a, "--workspace="):
			workspace = strings.TrimPrefix(a, "--workspace=")
		case a == "--profile":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--profile 需要参数（desktop 只有 web）")
				return 2
			}
			i++
			profileName = args[i]
		case strings.HasPrefix(a, "--profile="):
			profileName = strings.TrimPrefix(a, "--profile=")
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
		if _, err := Bundle(rest[1], platform, force, install, skipInstall); err != nil {
			fmt.Fprintf(os.Stderr, "bundle 失败：%v\n", err)
			return 1
		}
		return 0
	case "plugin":
		if len(rest) < 2 || rest[1] != "add" {
			fmt.Fprintln(os.Stderr, "用法：deepseek-harness-desktop plugin add [--workspace=<path>] <package...>")
			return 2
		}
		if profileName != "" && profileName != config.ProfileName {
			fmt.Fprintf(os.Stderr, "desktop 只有 %s profile（--profile=%s 无效）\n", config.ProfileName, profileName)
			return 2
		}
		pkgs := rest[2:]
		if len(pkgs) == 0 {
			fmt.Fprintln(os.Stderr, "用法：deepseek-harness-desktop plugin add [--workspace=<path>] <package...>")
			return 2
		}
		ws := workspace
		if ws == "" {
			ws = "." // 缺省当前目录（与 dsh plugin 在 profile 目录操作一致）
		}
		if err := PluginAdd(ws, pkgs, skipInstall); err != nil {
			fmt.Fprintf(os.Stderr, "plugin add 失败：%v\n", err)
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

// Bundle 执行一次完整打包（依赖闭包 → SEA → 壳 → 平台组装），返回产物
// 路径。
//
// 默认基于构建缓存：工作区内容（package.json / cordis.patch.yml /
// pnpm-workspace.yaml / .npmrc / 图标 / pnpm-lock.yaml 等）与上次打包一致
// 时直接复用已有产物；--force 忽略缓存全新打包。
func Bundle(ws, platform string, force, install, skipInstall bool) (string, error) {
	if err := checkPlatform(platform); err != nil {
		return "", err
	}
	_, ws, cfg, err := loadWorkspace(ws)
	if err != nil {
		return "", err
	}

	// 工作区 .gitignore：被忽略的内容（构建产物、缓存等）不参与 hash，
	// 也不进 DSH_HOME 种子（bundle 内部同规则）。
	gi, err := gitignore.Load(ws)
	if err != nil {
		return "", fmt.Errorf("load .gitignore: %w", err)
	}
	hashIgnored := gi.Ignored

	// 构建缓存：工作区 dir hash + 闭包指纹 + 平台。产物位于工作区 target/ 下。
	statePath := filepath.Join(config.BuildDir(ws, cfg), ".build-state.json")
	if !force {
		wsHash, err := workspaceHash(ws, hashSkip, hashIgnored)
		if err != nil {
			return "", fmt.Errorf("计算工作区 hash: %w", err)
		}
		if state, err := os.ReadFile(statePath); err == nil {
			var st buildState
			if json.Unmarshal(state, &st) == nil && st.Hash == wsHash && st.Platform == platformName() {
				appRoot := bundle.AppRoot(ws, cfg)
				if dirExists(appRoot) {
					fmt.Printf("==> 无变化（%s），复用 %s\n", st.Hash[:12], appRoot)
					if install {
						if err := bundle.Install(appRoot, cfg); err != nil {
							return "", err
						}
					}
					return appRoot, nil
				}
			}
		}
	}

	fmt.Printf("==> 打包 %s（%s %s）\n", cfg.Name, config.ProfileName, cfg.Version)

	// 1) SEA 后端。
	seaExe, err := sea.Build(ws, cfg, skipInstall)
	if err != nil {
		return "", err
	}
	fmt.Printf("==> SEA 后端: %s\n", seaExe)

	// 2) 壳二进制（构建输入由 shellsrc 内嵌，脱离源码树）。
	shellBin, err := buildShell(ws, cfg)
	if err != nil {
		return "", err
	}

	// 3) 平台组装。
	appRoot, err := bundle.Assemble(bundle.Inputs{
		Workspace: ws,
		Cfg:       cfg,
		SeaExe:    seaExe,
		ShellBin:  shellBin,
	})
	if err != nil {
		return "", err
	}
	fmt.Printf("==> 产物: %s\n", appRoot)

	// 记录构建状态。
	wsHash, err := workspaceHash(ws, hashSkip, hashIgnored)
	if err == nil {
		st := buildState{Hash: wsHash, Platform: platformName()}
		if raw, err := json.Marshal(st); err == nil {
			if err := os.MkdirAll(config.BuildDir(ws, cfg), 0o755); err == nil {
				_ = os.WriteFile(statePath, raw, 0o644)
			}
		}
	}

	// 4) 安装（可选）。
	if install {
		if err := bundle.Install(appRoot, cfg); err != nil {
			return "", err
		}
	}
	return appRoot, nil
}

// buildState 是构建缓存记录。
type buildState struct {
	Hash     string `json:"hash"`
	Platform string `json:"platform"`
}

// workspaceHash 计算工作区构建缓存指纹：工程文件 dir hash + 闭包顶层包
// 清单指纹。node_modules 不在 dir hash 内（体积与稳定性），pnpm install
// 导致的闭包变化（增删/升级包）单独纳入指纹，避免复用与当前闭包不一致
// 的旧产物——SEA 闭包缺包时 tsdown 把解析不到的依赖留作裸导入，产物
// 启动即崩（ERR_UNKNOWN_BUILTIN_MODULE）。
func workspaceHash(ws string, hashSkip map[string]bool, hashIgnored func(rel string, isDir bool) bool) (string, error) {
	h, err := fsutil.DirHash(ws, hashSkip, hashIgnored)
	if err != nil {
		return "", err
	}
	fp, err := profile.ClosureFingerprint(ws)
	if err != nil {
		return "", err
	}
	return h + ":" + fp, nil
}

// hashSkip 是工作区 dir hash 排除的名字（安装簿记与运行时生成物；
// pnpm-lock.yaml 锁定依赖闭包，必须参与 hash）。构建产物（如 target/）
// 不在此列——由工作区 .gitignore 表达，hash 遵循它排除（见 Bundle）。
// node_modules 由 workspaceHash 以闭包指纹单独纳入。
var hashSkip = map[string]bool{
	".git":         true,
	"node_modules": true,
	".store":       true,
	".DS_Store":    true,
	"cordis.yml":   true,
}

// platformName 返回当前平台的 canonical 名（os/arch）。
func platformName() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

// Dev 基于工作区直接起一个 dsh web 并打开浏览器页面（不组装桌面应用，
// 无 Wails 壳）。等价于官方流程：
//
//	DSH_HOME=<xdg.DataHome>/<name> dsh web --patch <ws>/cordis.patch.yml
//
// 实现：DSH_HOME 固定为 XDG 数据目录（与打包后应用运行时一致），
// $DSH_HOME/profiles/web 符号链接指向工作区——dsh 直接从工作区读
// package.json（bundles）与 cordis.patch.yml（patch 层），工作区的
// pnpm install 结果直接可见。
func Dev(ws string, skipInstall bool) error {
	_, ws, cfg, err := loadWorkspace(ws)
	if err != nil {
		return err
	}

	fmt.Printf("==> dev %s（%s %s）\n", cfg.Name, config.ProfileName, cfg.Version)

	// 1) 工程文件兜底 + 未安装时 pnpm install（复用工作区已有安装）。
	if _, err := profile.Ensure(ws, skipInstall); err != nil {
		return err
	}

	// 2) 构造运行时 DSH_HOME：xdg.DataHome/<name>，profiles/web → 工作区。
	homeDir := filepath.Join(xdg.DataHome, cfg.Name)
	if err := os.MkdirAll(filepath.Join(homeDir, "profiles"), 0o755); err != nil {
		return err
	}
	profileLink := filepath.Join(homeDir, "profiles", config.ProfileName)
	if _, err := os.Lstat(profileLink); os.IsNotExist(err) {
		if err := os.Symlink(ws, profileLink); err != nil {
			return fmt.Errorf("构造 profiles/web 链接: %w", err)
		}
	}

	// 3) 启动 dsh web（工作区闭包里的 dsh），解析就绪 URL。
	dshBin := filepath.Join(ws, "node_modules", ".bin", "dsh")
	if _, err := os.Stat(dshBin); err != nil {
		return fmt.Errorf("工作区未安装 dsh（%s）；先 pnpm install 或去掉 --skip-install", dshBin)
	}
	url, err := runWeb(dshBin, homeDir)
	if err != nil {
		return err
	}

	// 4) 打开浏览器页面。
	fmt.Printf("==> 打开 %s（Ctrl+C 退出）\n", url)
	if err := openURL(url); err != nil {
		fmt.Fprintf(os.Stderr, "[warn] 打开浏览器失败: %v\n", err)
	}
	return nil
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

// dirExists 报告路径是否为已存在的目录。
func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
