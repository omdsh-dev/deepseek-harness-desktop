// `plugin add` 子命令：代理 dsh 的 plugin add，但不安装到全局
// DSH_HOME，而是修改工作区（bundle workspace）的 dsh.profile.bundles。
//
// 复用 dev 的运行时布局：DSH_HOME 为工作区本地临时目录 .dsh-store，
// $DSH_HOME/profiles/web 符号链接指向工作区；随后调用工作区闭包里的
// `dsh plugin --profile web add <pkg...>`——dsh 在工作区跑 pnpm add，
// 成功后在 package.json 里 reconcile dsh.profile.bundles（依赖中声明
// dsh.bundle.patch 的包自动入层，被移除/失去声明的包出层），全程不触碰
// 全局 DSH_HOME。
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/omdsh-dev/deepseek-harness-desktop/internal/config"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/pm"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/profile"
)

// PluginAdd 代理 `dsh plugin --profile web add <pkg...>`，目标为工作区。
// pnpm add 与 bundles reconcile 均由工作区闭包里的 dsh 完成（与官方
// 流程一致），本命令只负责 DSH_HOME 布局与进程调用。
func PluginAdd(ws string, pkgs []string, skipInstall bool) error {
	_, ws, _, err := loadWorkspace(ws)
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		return fmt.Errorf("缺插件包名；用法：deepseek-harness-desktop plugin add [--workspace=<path>] <package...>")
	}

	// 1) 工程文件兜底 + 未安装时 pnpm install（复用工作区已有安装）。
	if _, err := profile.Ensure(ws, skipInstall); err != nil {
		return err
	}

	// 2) 构造 dev 运行时 DSH_HOME（与 dev 一致）：工作区 .dsh-store，
	//    profiles/web → 工作区（只补缺失，不重建——dev 可能正在运行）。
	//    dsh 的 plugin 命令在此布局下操作工作区。
	homeDir, err := ensureDevHome(ws, false)
	if err != nil {
		return err
	}

	// 3) 调用工作区闭包里的 dsh（与 dev 相同）。
	dshBin := filepath.Join(ws, "node_modules", ".bin", "dsh")
	if _, err := os.Stat(dshBin); err != nil {
		return fmt.Errorf("工作区未安装 dsh（%s）；先 pnpm install 或去掉 --skip-install", dshBin)
	}

	fmt.Printf("==> plugin add %s（工作区 %s）\n", strings.Join(pkgs, " "), ws)
	args := append([]string{"plugin", "--profile", config.ProfileName, "add"}, pkgs...)
	cmd := exec.Command(dshBin, args...)
	cmd.Env = withEnv(os.Environ(), "DSH_HOME", homeDir)
	// dsh 内部用 PATH 上的 pnpm 跑 add：优先放入真实 pnpm（mise 安装），
	// 避免命中 nub shim（配置语义不一致，见 internal/pm）。
	if bin, err := pm.Bin(); err == nil {
		cmd.Env = prependPath(cmd.Env, filepath.Dir(bin))
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dsh plugin add: %w", err)
	}

	// 4) 汇报 reconcile 后的 bundle 列表。
	if cfg, err := config.Load(ws); err == nil {
		fmt.Printf("==> bundles: [%s]\n", strings.Join(cfg.Bundles, ", "))
	}
	return nil
}

// ensureProfileLink 确保 profiles/web 符号链接指向工作区：不存在则创建；
// 已存在但指向别处时拒绝（避免把插件装进别的目录）。
func ensureProfileLink(link, ws string) error {
	info, err := os.Lstat(link)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s 已存在但不是符号链接；请手动处理后再试", link)
		}
		got, err := filepath.EvalSymlinks(link)
		if err != nil {
			return fmt.Errorf("解析 %s: %w", link, err)
		}
		want, err := filepath.EvalSymlinks(ws)
		if err != nil {
			return fmt.Errorf("解析 %s: %w", ws, err)
		}
		if got != want {
			return fmt.Errorf("%s 指向 %s，不是工作区 %s；请手动处理后再试", link, got, ws)
		}
		return nil
	case os.IsNotExist(err):
		if err := os.Symlink(ws, link); err != nil {
			return fmt.Errorf("构造 profiles/web 链接: %w", err)
		}
		return nil
	default:
		return err
	}
}

// withEnv 返回 env 的副本，其中 key 的值替换为 value（先移除旧条目再
// 追加，保证生效——父进程环境里可能已有同名变量，如 DSH_HOME）。
func withEnv(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	prefix := key + "="
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return append(out, prefix+value)
}

// prependPath 返回 env 的副本，把 dir 放到 PATH 最前（供 dsh 调起的
// pnpm 解析到真实二进制）。
func prependPath(env []string, dir string) []string {
	old := ""
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			old = strings.TrimPrefix(e, "PATH=")
			continue
		}
		out = append(out, e)
	}
	return append(out, "PATH="+dir+string(os.PathListSeparator)+old)
}
