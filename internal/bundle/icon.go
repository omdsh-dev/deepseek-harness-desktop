package bundle

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dsh-external/deepseek-harness-desktop/internal/config"
	"github.com/dsh-external/deepseek-harness-desktop/internal/fsutil"
	"github.com/dsh-external/deepseek-harness-desktop/internal/tools"
	xdraw "golang.org/x/image/draw"
)

// iconMJS 用工具链里的 sharp（libvips + librsvg）把 SVG 渲染为 1024x1024
// 白底 PNG（currentColor 由 librsvg 按黑色解析）。resvg 无法解析部分 SVG
// 特性，sharp 的渲染路径与旧构建脚本一致。
const iconMJS = `import sharp from 'sharp'
const [src, dst] = process.argv.slice(2)
await sharp(src, { density: 1440 })
  .resize(1024, 1024, { fit: 'contain', background: { r: 255, g: 255, b: 255, alpha: 1 } })
  .png()
  .toFile(dst)
`

// renderIcon1024 把工作区图标的 SVG 渲染为 1024x1024 PNG（工具链 sharp）。
// 返回 PNG 文件路径（target/<name>/icon-1024.png）。
func renderIcon1024(in Inputs) (string, error) {
	svgPath := filepath.Join(in.Workspace, in.Cfg.Desktop.Icon)
	if _, err := os.Stat(svgPath); err != nil {
		return "", fmt.Errorf("icon 源缺失 %s: %w", svgPath, err)
	}
	toolsDir, err := tools.Ensure(in.Root)
	if err != nil {
		return "", err
	}
	script := filepath.Join(toolsDir, "icon.mjs")
	if err := os.WriteFile(script, []byte(iconMJS), 0o644); err != nil {
		return "", err
	}
	outDir := config.BuildDir(in.Root, in.Cfg)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(outDir, "icon-1024.png")
	node, err := tools.Node()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(node, script, svgPath, out)
	cmd.Dir = toolsDir // 让 import sharp 解析到 tools/node_modules
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("渲染图标: %w", err)
	}
	return out, nil
}

// resizePNG 把 PNG 数据缩放到 size x size。
func resizePNG(data []byte, size int) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.NearestNeighbor.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writePng 写出缩放后的 PNG。
func writePng(path string, data []byte, size int) error {
	out, err := resizePNG(data, size)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// makeIconset 生成 macOS iconset（16–512 @1x/@2x）。
func makeIconset(srcPNG string, iconset string) error {
	if err := os.MkdirAll(iconset, 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(srcPNG)
	if err != nil {
		return err
	}
	for _, s := range []int{16, 32, 128, 256, 512} {
		if err := writePng(filepath.Join(iconset, fmt.Sprintf("icon_%dx%d.png", s, s)), data, s); err != nil {
			return err
		}
		if err := writePng(filepath.Join(iconset, fmt.Sprintf("icon_%dx%d@2x.png", s, s)), data, s*2); err != nil {
			return err
		}
	}
	return nil
}

// makeIcns 用系统 iconutil 把 iconset 打包为 icns。
func makeIcns(iconset, out string) error {
	cmd := exec.Command("iconutil", "-c", "icns", iconset, "-o", out)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("iconutil: %w", err)
	}
	return nil
}

// makeHicolor 生成 freedesktop hicolor 多尺寸图标集（16–512 + scalable SVG）。
func makeHicolor(srcPNG, srcSVG, iconsRoot, iconName string) error {
	data, err := os.ReadFile(srcPNG)
	if err != nil {
		return err
	}
	for _, s := range []int{16, 22, 24, 32, 48, 64, 128, 256, 512} {
		dir := filepath.Join(iconsRoot, "hicolor", fmt.Sprintf("%dx%d", s, s), "apps")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := writePng(filepath.Join(dir, iconName+".png"), data, s); err != nil {
			return err
		}
	}
	scalable := filepath.Join(iconsRoot, "hicolor", "scalable", "apps")
	if err := os.MkdirAll(scalable, 0o755); err != nil {
		return err
	}
	return fsutil.CopyFile(srcSVG, filepath.Join(scalable, iconName+".svg"))
}

// makeIco 组装多尺寸 PNG 内嵌的 ICO（Vista+ 支持 PNG 压缩条目）。
func makeIco(srcPNG string, out string) error {
	data, err := os.ReadFile(srcPNG)
	if err != nil {
		return err
	}
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	pngs := make([][]byte, 0, len(sizes))
	for _, s := range sizes {
		p, err := resizePNG(data, s)
		if err != nil {
			return err
		}
		pngs = append(pngs, p)
	}
	header := make([]byte, 6)
	header[2] = 1 // type: icon
	header[4] = byte(len(pngs))
	entries := make([][]byte, 0, len(pngs))
	offset := 6 + 16*len(pngs)
	for i, s := range sizes {
		e := make([]byte, 16)
		if s >= 256 {
			e[0], e[1] = 0, 0
		} else {
			e[0], e[1] = byte(s), byte(s)
		}
		e[4], e[5] = 1, 0 // planes
		e[6], e[7] = 32, 0 // bpp
		lePut32(e[8:12], uint32(len(pngs[i])))
		lePut32(e[12:16], uint32(offset))
		offset += len(pngs[i])
		entries = append(entries, e)
	}
	var buf bytes.Buffer
	buf.Write(header)
	for _, e := range entries {
		buf.Write(e)
	}
	for _, p := range pngs {
		buf.Write(p)
	}
	return os.WriteFile(out, buf.Bytes(), 0o644)
}

func lePut32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

// iconFor 按平台生成应用图标：返回图标文件路径（无图标配置时返回空串）。
func iconFor(in Inputs, destDir string, platform string) (string, error) {
	if in.Cfg.Desktop.Icon == "" {
		return "", nil
	}
	png1024, err := renderIcon1024(in)
	if err != nil {
		return "", err
	}
	switch platform {
	case "darwin":
		iconset := filepath.Join(destDir, "dsh.iconset")
		if err := makeIconset(png1024, iconset); err != nil {
			return "", err
		}
		out := filepath.Join(destDir, "dsh.icns")
		if err := makeIcns(iconset, out); err != nil {
			return "", err
		}
		return out, nil
	case "linux":
		out := filepath.Join(destDir, "icons")
		if err := makeHicolor(png1024, filepath.Join(in.Workspace, in.Cfg.Desktop.Icon), out, "dsh"); err != nil {
			return "", err
		}
		return out, nil
	case "windows":
		out := filepath.Join(destDir, "dsh.ico")
		if err := makeIco(png1024, out); err != nil {
			return "", err
		}
		return out, nil
	}
	return "", nil
}
