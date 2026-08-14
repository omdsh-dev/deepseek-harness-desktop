// 用工具链里的 sharp（libvips + librsvg）把 SVG 渲染为 1024x1024 白底
// PNG（currentColor 由 librsvg 按黑色解析）。resvg 无法解析部分 SVG
// 特性，sharp 的渲染路径与旧构建脚本一致。
import sharp from 'sharp'
const [src, dst] = process.argv.slice(2)
await sharp(src, { density: 1440 })
  .resize(1024, 1024, { fit: 'contain', background: { r: 255, g: 255, b: 255, alpha: 1 } })
  .png()
  .toFile(dst)
