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
// dshHome 策略把种子强制落位到目标 DSH_HOME（xdg 策略即 xdg.DataHome/<name>）。
const seedDirName = "dsh-home"

// seedHashName 是种子里记录工作区内容 hash 的指纹文件名（bundle 时
// 写入，壳启动时比对）。
const seedHashName = ".seed-hash"

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
//	                               home 一致。每次启动把 bundle 内种子
//	                               强制落位为实体 profile（指纹比对，
//	                               见 ensureSeed）；
//	<绝对路径>                    — 固定使用该目录；同样强制种子落位；
//	env                          — 返回空串，不设置 DSH_HOME（继承环境）。
//
// dev 模式在工作区本地临时目录 .dsh-store 里把 profiles/web 创建为指向
// 工作区的符号链接（运行时直连工作区），不写全局 DSH_HOME；打包 app
// 必须独立于工作区——启动时强制 profiles/web 为实体种子（dev/旧版本
// 残留的 symlink 或旧实体拷贝都被替换，见 ensureSeed）。返回空串表示
// 调用方不设置 DSH_HOME。
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

// ensureSeed 强制目标 DSH_HOME 的 profile 为来自 bundle 种子的实体副本：
// dev/旧版本残留的符号链接与旧实体拷贝，与种子指纹（.seed-hash，打包时
// 工作区内容 hash）不一致时移除并用种子覆盖——profile 定义随应用更新，
// 应用永远以打包时的工作区内容为准。指纹一致（同一版本正常启动）跳过
// 复制，避免每次启动全量复制 node_modules 闭包。用户数据（sessions/
// settings.yaml 等）位于 home 根，不受 profile 替换影响。
func ensureSeed(seed, dst, profile string) error {
	if !dirExists(seed) {
		return nil
	}
	profileDir := filepath.Join(dst, "profiles", profile)
	seedHash := readSeedHash(filepath.Join(seed, "profiles", profile, seedHashName))
	if seedHash != "" {
		if info, err := os.Lstat(profileDir); err == nil && info.Mode()&os.ModeSymlink == 0 {
			if readSeedHash(filepath.Join(profileDir, seedHashName)) == seedHash {
				return nil // 已是当前种子的实体副本
			}
		}
	}
	if _, err := os.Lstat(profileDir); err == nil {
		if err := os.RemoveAll(profileDir); err != nil {
			return fmt.Errorf("移除旧 profile %s: %w", profileDir, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := copySeed(seed, dst); err != nil {
		return fmt.Errorf("拷贝 dsh-home 种子到 %s: %w", dst, err)
	}
	return nil
}

// readSeedHash 读取 .seed-hash 指纹内容（不存在返回空串）。
func readSeedHash(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
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
