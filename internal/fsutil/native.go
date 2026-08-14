package fsutil

import (
	"path"
	"runtime"
	"strings"
)

// nativePkgFamilies 是带平台变体的原生包家族前缀（optionalDependencies
// 平台包，如 @img/sharp-<plat>-<arch>、@koromix/koffi-<plat>-<arch>）。
// 复制闭包时只保留当前平台的变体。
var nativePkgFamilies = []string{
	"sharp",
	"sharp-libvips",
	"koffi",
	"ripgrep",
	"node-addon-require-builtin",
	"node-addon-internal-loader",
}

// nativePlatform 是当前平台的 prebuilds 目录名 / 平台包后缀名
// （npm 生态命名：darwin-arm64、linux-x64、win32-x64 等）。
var nativePlatform = func() string {
	osName := runtime.GOOS
	if osName == "windows" {
		osName = "win32"
	}
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x64"
	case "386":
		arch = "ia32"
	}
	return osName + "-" + arch
}()

// NativeSkip 报告复制 node_modules 闭包时是否应跳过该条目（与当前平台
// 无关的原生二进制）：
//
//   - prebuilds/<platform>-<arch>/ 目录：node-pty 等包内自带的全平台
//     预编译二进制，只保留当前平台目录；
//   - 平台变体包（@img/sharp-*、@koromix/koffi-* 等）：pnpm 默认按平台
//     安装 optionalDependencies，此处兜底过滤其他平台的变体。
//
// rel 是相对 node_modules 的 / 分隔路径，与 CopyDirDeref 的 ignored
// 回调约定一致（目录命中即整体跳过）。
func NativeSkip(rel string, isDir bool) bool {
	segs := strings.Split(rel, "/")
	// prebuilds/<platform>-<arch>/（任意包内，目录本身命中即跳过子树；
	// 非平台名的散文件不误伤）。
	if isDir {
		for i, s := range segs {
			if s == "prebuilds" && i+1 == len(segs)-1 {
				dir := segs[i+1]
				parts := strings.Split(dir, "-")
				if len(parts) == 2 && isKnownPlatform(parts[0]) && isKnownArch(parts[1]) {
					return dir != nativePlatform
				}
			}
		}
	}
	// 平台变体包：@scope/name 或 name，后缀匹配 family-<plat>-<arch>。
	base := path.Base(rel)
	for _, fam := range nativePkgFamilies {
		if !strings.HasPrefix(base, fam+"-") {
			continue
		}
		head := strings.TrimPrefix(base, fam+"-")
		// 去掉编译器后缀（-gnu/-musl/-msvc 等），剩 <plat>-<arch>。
		for _, extra := range []string{"-gnu", "-musl", "-msvc"} {
			if strings.HasSuffix(head, extra) {
				head = strings.TrimSuffix(head, extra)
				break
			}
		}
		parts := strings.Split(head, "-")
		if len(parts) != 2 || !isKnownPlatform(parts[0]) || !isKnownArch(parts[1]) {
			continue // 家族名巧合，非平台变体
		}
		return head != nativePlatform
	}
	return false
}

func isKnownPlatform(s string) bool {
	switch s {
	case "darwin", "linux", "linuxmusl", "win32":
		return true
	}
	return false
}

func isKnownArch(s string) bool {
	switch s {
	case "x64", "arm64", "ia32", "arm", "riscv64", "ppc64", "s390x":
		return true
	}
	return false
}
