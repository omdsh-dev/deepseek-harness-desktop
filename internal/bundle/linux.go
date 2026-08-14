package bundle

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/omdsh-dev/deepseek-harness-desktop/internal/config"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/fsutil"
)

// assembleLinux 组装 Linux 应用 target/<name>/linux/<Name>/：
//
//	bin/{dsh-shell,dsh-server,appconfig.json}
//	config/ node_modules/ package.json dsh-home/
//	share/icons/hicolor/（16–512 + scalable SVG）
//
// 完成后打包 target/<name>/linux/<Name>.tar.gz（顶层目录 <Name>/）。
func assembleLinux(in Inputs) (string, error) {
	root := filepath.Join(config.BuildDir(in.Workspace, in.Cfg), "linux")
	appRoot := filepath.Join(root, in.Cfg.Name)
	if err := fsutil.RemoveAll(root); err != nil {
		return "", err
	}
	if _, err := assembleLayout(in, appRoot); err != nil {
		return "", err
	}

	// 图标（可选）：share/icons/hicolor/。
	if in.Cfg.Desktop.Icon != "" {
		iconsRoot := filepath.Join(appRoot, "share", "icons")
		if err := os.MkdirAll(iconsRoot, 0o755); err != nil {
			return "", err
		}
		if _, err := iconFor(in, iconsRoot, "linux"); err != nil {
			return "", err
		}
	}

	// 归档 tar.gz。
	tarPath := filepath.Join(root, in.Cfg.Name+".tar.gz")
	if err := tarGz(appRoot, tarPath, in.Cfg.Name); err != nil {
		return "", fmt.Errorf("tar.gz: %w", err)
	}
	return appRoot, nil
}

// tarGz 把 dir 打包为 tar.gz，归档内顶层目录名为 topName。
func tarGz(dir, out, topName string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(topName, rel))
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		hdr.Name = name
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(tw, in)
		return err
	})
}
