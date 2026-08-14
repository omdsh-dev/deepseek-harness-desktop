package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

// seedDirName 是 bundle 内 DSH_HOME 种子目录名（位于壳可执行文件上一级）。
// 打包时 CLI 把构建出的 profile 布局（profiles/web/）复制到这里；运行时按
// dshHome 策略把种子内容合并进目标 DSH_HOME（xdg 策略即 xdg.DataHome/<name>）。
const seedDirName = "dsh-home"

// 复制时排除的目录（安装簿记与 store，非 dsh 运行时所需）。
var seedSkipDirs = map[string]bool{
	".nub-store": true,
	".store":     true,
	".nub":       true,
}

// resolveDSHHome 按 appconfig 的 dshHome 策略解析 DSH_HOME：
//
//	DSH_APP_DSH_HOME（环境变量） — 显式覆盖，原样返回（开发/测试用）；
//	xdg（默认）                  — XDG 数据目录：xdg.DataHome/<name>
//	                               （Linux ~/.local/share、macOS
//	                               ~/Library/Application Support 等，见
//	                               github.com/adrg/xdg），与 dev 的运行时
//	                               home 一致。首次启动把 bundle 内种子
//	                               拷贝进去，之后读写都在拷贝上；
//	<绝对路径>                    — 固定使用该目录；若 profiles/web 缺失且
//	                               种子存在，从种子补齐缺失文件；
//	env                          — 返回空串，不设置 DSH_HOME（继承环境）。
//
// dev 模式会把 profiles/web 创建为指向工作区的符号链接（运行时直连
// 工作区）；打包 app 必须独立于工作区——检测到 symlink 时移除并复制
// 实体种子（见 ensureSeed）。返回空串表示调用方不设置 DSH_HOME。
func resolveDSHHome(cfg appConfig, exeDir string) (string, error) {
	if v := os.Getenv("DSH_APP_DSH_HOME"); v != "" {
		return v, nil
	}
	seed := filepath.Join(exeDir, "..", seedDirName)

	switch cfg.DSHHome {
	case "env":
		return "", nil
	case "xdg":
		dst := filepath.Join(xdg.DataHome, cfg.Name)
		if err := ensureSeed(seed, dst, cfg.Profile); err != nil {
			return "", err
		}
		return dst, nil
	default:
		dst := cfg.DSHHome
		if !filepath.IsAbs(dst) {
			return "", fmt.Errorf("dshHome 必须是 xdg / env / 绝对路径，得到 %q", cfg.DSHHome)
		}
		if err := ensureSeed(seed, dst, cfg.Profile); err != nil {
			return "", err
		}
		return dst, nil
	}
}

// ensureSeed 确保目标 DSH_HOME 的 profile 是实体种子副本：dev 模式留下
// 的 profiles/web 符号链接（指向工作区）不是有效种子——移除后从 bundle
// 内种子复制实体，使 app 独立于工作区（工作区 node_modules 缺失/变更
// 不影响 app 启动）。用户数据（sessions/storages 等）位于 home 根，
// 不受 profile 替换影响。
func ensureSeed(seed, dst, profile string) error {
	if !dirExists(seed) {
		return nil
	}
	profileDir := filepath.Join(dst, "profiles", profile)
	if info, err := os.Lstat(profileDir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(profileDir); err != nil {
			return fmt.Errorf("移除 dev symlink %s: %w", profileDir, err)
		}
	}
	if dirExists(profileDir) {
		return nil
	}
	if err := copySeed(seed, dst); err != nil {
		return fmt.Errorf("首次启动拷贝 dsh-home 种子到 %s: %w", dst, err)
	}
	return nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// copySeed 把种子目录递归复制到 dst（跳过安装簿记目录）。目标已存在的
// 文件不覆盖（用户数据优先），只补缺失。
func copySeed(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if seedSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		}
		// 只补缺失文件，不覆盖用户已有数据。
		if _, err := os.Stat(target); err == nil {
			return nil
		}
		return copyFileMode(path, target)
	})
}

func copyFileMode(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
