#!/usr/bin/env zx
/**
 * 组装 Linux 桌面应用 target/linux/DSH/：
 *   bin/dsh-server    SEA 可执行（Linux 上由 `just sea` 产出）
 *   bin/dsh-shell     Wails 壳（justfile 中 go build 输出到该位置）
 *   config/ node_modules/ package.json   运行时资源（dsh-server 从 bin 上一级解析）
 *   share/icons/dsh.png   图标（resvg 渲染 icon.svg，Linux 无 sips/iconutil）
 * 组装后打包 target/linux/DSH.tar.gz（顶层目录 DSH/）。
 *
 * 仅在 Linux 上执行：SEA（Node --build-sea）与 Wails 壳（cgo WebKitGTK）
 * 均不支持交叉编译，产物必须由 Linux 主机构建。本脚本不区分平台，在
 * macOS 上干跑会得到 macOS 的 dsh-server（仅用于验证组装逻辑）。
 *
 * 用法：`zx scripts/make-linux-app.mts`（前置：sea 已构建；壳由 justfile 输出）
 */
import { $, fs, path } from 'zx'
import sharp from 'sharp'

const ROOT = path.join(import.meta.dirname, '..')
const SEA = path.join(ROOT, 'target/sea')
const LINUX = path.join(ROOT, 'target/linux')
const APP = path.join(LINUX, 'DSH')
const ICON_SRC = path.join(ROOT, 'apps/dsh-desktop/assets/icon.svg')

if (!fs.existsSync(path.join(SEA, 'bin/dsh'))) {
  console.error(`未找到 SEA 产物：${path.join(SEA, 'bin/dsh')}（先运行 just sea）`)
  process.exit(1)
}

fs.rmSync(LINUX, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 })
fs.mkdirSync(path.join(APP, 'bin'), { recursive: true })
fs.mkdirSync(path.join(APP, 'share/icons'), { recursive: true })

fs.copyFileSync(path.join(SEA, 'bin/dsh'), path.join(APP, 'bin/dsh-server'))
fs.cpSync(path.join(SEA, 'config'), path.join(APP, 'config'), { recursive: true })
fs.cpSync(path.join(SEA, 'node_modules'), path.join(APP, 'node_modules'), { recursive: true })
fs.copyFileSync(path.join(SEA, 'package.json'), path.join(APP, 'package.json'))
fs.chmodSync(path.join(APP, 'bin/dsh-server'), 0o755)

// 图标：sharp（libvips + librsvg）把 icon.svg 渲染到 512x512 白底 png。
// 不依赖 sips/iconutil（macOS 专用）；resvg-js 无法解析该 SVG（见 icon.mts
// 用 sips 的原因），sharp 经 librsvg 可渲染。高 density 渲染后缩到画布，
// 保持矢量清晰度；currentColor 由 librsvg 按黑色解析，观感对齐 icns。
const svg = fs.readFileSync(ICON_SRC)
await sharp(svg, { density: 1440 })
  .resize(512, 512, { fit: 'contain', background: { r: 255, g: 255, b: 255, alpha: 1 } })
  .png()
  .toFile(path.join(APP, 'share/icons/dsh.png'))

await $`tar -czf ${path.join(LINUX, 'DSH.tar.gz')} -C ${LINUX} DSH`

console.log(`已生成：${APP}`)
console.log(`启动：${path.join(APP, 'bin/dsh-shell')}`)
