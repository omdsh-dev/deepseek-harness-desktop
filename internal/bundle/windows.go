package bundle

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/omdsh-dev/deepseek-harness-desktop/internal/config"
	"github.com/omdsh-dev/deepseek-harness-desktop/internal/fsutil"
)

// assembleWindows 组装 Windows 应用 target/<name>/windows/<Name>/：
//
//	bin/{dsh-shell.exe,dsh-server.exe,appconfig.json}
//	config/ node_modules/ package.json dsh-home/
//	dsh.ico
//
// 完成后打包 target/<name>/windows/<Name>.zip（顶层目录 <Name>/）。
func assembleWindows(in Inputs) (string, error) {
	root := filepath.Join(config.BuildDir(in.Workspace, in.Cfg), "windows")
	appRoot := filepath.Join(root, in.Cfg.Name)
	if err := fsutil.RemoveAll(root); err != nil {
		return "", err
	}
	if _, err := assembleLayout(in, appRoot); err != nil {
		return "", err
	}

	// 图标（可选）：dsh.ico（多尺寸 PNG 内嵌，Vista+）。
	if in.Cfg.Desktop.Icon != "" {
		if _, err := iconFor(in, appRoot, "windows"); err != nil {
			return "", err
		}
	}

	// 归档 zip。
	zipPath := filepath.Join(root, in.Cfg.Name+".zip")
	if err := zipDir(appRoot, zipPath, in.Cfg.Name); err != nil {
		return "", fmt.Errorf("zip: %w", err)
	}
	return appRoot, nil
}

// zipDir 把 dir 打包为 zip，归档内顶层目录名为 topName。
func zipDir(dir, out, topName string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()

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
		if info.IsDir() {
			_, err := zw.Create(name + "/")
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// 不打包 symlink 实体（bundle 内不应有链接）。
			return nil
		}
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(w, in)
		return err
	})
}
