// Package fsutil 提供构建与运行时共用的文件复制工具。
package fsutil

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CopyFile 复制单个文件并保留可执行位。
func CopyFile(src, dst string) error {
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

// CopyDir 递归复制目录（不解引用 symlink，按原样复制链接）。
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		return CopyFile(path, target)
	})
}

// CopyDirDeref 递归复制目录并把 symlink 解引用为实体（保留可执行位）。
// skip 里的名字（任意层级的文件或目录）整体跳过；ignored 非 nil 时，
// 相对路径（/ 分隔）被其判为忽略的条目也跳过（用于遵循 .gitignore）。
// 若目标目录位于源内部（例如产物写回工作区），复制会跳过目标自身的
// 子树，避免把复制中的内容递归包含进来（不依赖任何目录名约定）。
func CopyDirDeref(src, dst string, skip map[string]bool, ignored func(rel string, isDir bool) bool) error {
	// 目标相对源的前缀（仅当目标在源内部时非空）。
	selfPrefix := ""
	if rel, err := filepath.Rel(src, dst); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel) {
		selfPrefix = filepath.ToSlash(rel)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		slashRel := filepath.ToSlash(rel)
		if selfPrefix != "" && (slashRel == selfPrefix || strings.HasPrefix(selfPrefix, slashRel+"/")) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if skip[filepath.Base(path)] || (ignored != nil && ignored(slashRel, info.IsDir())) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if !filepath.IsAbs(link) {
				link = filepath.Join(filepath.Dir(path), link)
			}
			return copyDeref(link, target)
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return CopyFile(path, target)
	})
}

// copyDeref 复制一个路径（若为 symlink 则解引用），目标按实体落盘。
func copyDeref(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(src)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(src), link)
		}
		return copyDeref(link, dst)
	}
	if info.IsDir() {
		return CopyDirDeref(src, dst, nil, nil)
	}
	return CopyFile(src, dst)
}

// RemoveAll 递归删除目录，带重试：macOS APFS 上删除大目录偶发
// ENOTEMPTY（目录项删除的瞬态竞争），重试可自愈。
func RemoveAll(path string) error {
	var err error
	for range 5 {
		err = os.RemoveAll(path)
		if err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

// DirHash 计算目录内容的稳定哈希：按相对路径排序后对每个文件
// sha256(相对路径 + 文件内容) 聚合。skip 里的名字（文件或目录，任意层级）
// 排除；ignored 非 nil 时，相对路径（/ 分隔）被其判为忽略的条目也排除
// （用于遵循 .gitignore）。用于构建缓存判断（输入无变化则跳过重新打包）。
func DirHash(root string, skip map[string]bool, ignored func(rel string, isDir bool) bool) (string, error) {
	h := sha256.New()
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if skip[d.Name()] || (ignored != nil && ignored(filepath.ToSlash(rel), d.IsDir())) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	for _, rel := range paths {
		f, err := os.Open(filepath.Join(root, rel))
		if err != nil {
			return "", err
		}
		io.WriteString(h, rel)
		h.Write([]byte{0})
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return "", err
		}
		f.Close()
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
