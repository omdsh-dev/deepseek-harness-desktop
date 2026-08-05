#!/usr/bin/env zx
/**
 * 从 apps/dsh-desktop/assets/icon.svg 生成 app 图标 target/dsh.icns（白底黑图）。
 *
 * 渲染链路（组合两个渲染器的可靠面）：
 *   1. sips 渲染 SVG 黑图（sips 对黑色 fill 可靠；对蓝色有缺陷）；
 *   2. sharp 读取黑图，缩放至画布 90% 并居中合成到 1024x1024 白底（5% padding），
 *      输出标准 PNG（sharp 编码可靠，sips 可读）；
 *   3. 生成 iconset 各尺寸，iconutil 打包 icns。
 *
 * 用法：`zx scripts/icon.mts`（前置：nub install 已含 @resvg/resvg-js）
 */
import { $, fs, path } from 'zx'
import sharp from 'sharp'

const ROOT = path.join(import.meta.dirname, '..')
const SRC = path.join(ROOT, 'apps/dsh-desktop/assets/icon.svg')
const TMP = path.join(ROOT, '.tmp/icon')
const PNG_CONTENT = path.join(TMP, 'content.png')
const PNG_1024 = path.join(TMP, 'icon-1024.png')
const ICONSET = path.join(TMP, 'icon.iconset')
const OUT = path.join(ROOT, 'target/dsh.icns')
const INK = '#000000'
const BG = { r: 255, g: 255, b: 255, alpha: 1 }
const CANVAS = 1024
const PAD_RATIO = 0.05

if (!fs.existsSync(SRC)) {
  console.error(`未找到图标源：${SRC}`)
  process.exit(1)
}

fs.rmSync(TMP, { recursive: true, force: true })
fs.mkdirSync(TMP, { recursive: true })
fs.mkdirSync(ICONSET, { recursive: true })

// 1) sips 渲染 SVG 为黑图（黑色 fill 可靠）
let svg = fs.readFileSync(SRC, 'utf8')
  .replace('fill="none"', '')
  .replace('fill="currentColor"', `fill="${INK}"`)
fs.writeFileSync(path.join(TMP, 'icon.svg'), svg)
await $`sips -s format png ${TMP}/icon.svg --out ${PNG_CONTENT}`.quiet()

// 2) sharp 缩放 + 白底合成（内容占画布 90%，居中 → 四周 5% padding）
const meta = await sharp(PNG_CONTENT).metadata()
const contentW = Math.round(CANVAS * (1 - 2 * PAD_RATIO))
const contentH = Math.round(contentW * meta.height / meta.width)
const content = await sharp(PNG_CONTENT)
  .resize(contentW, contentH, { fit: 'fill' })
  .toBuffer()
await sharp({
  create: {
    width: CANVAS, height: CANVAS, channels: 4, background: BG,
  },
})
  .composite([{ input: content, left: Math.round((CANVAS - contentW) / 2), top: Math.round((CANVAS - contentH) / 2) }])
  .png()
  .toFile(PNG_1024)

// 3) 生成 iconset 各尺寸（sips 缩放标准 PNG 可靠）
for (const s of [16, 32, 128, 256, 512]) {
  await $`sips -z ${s} ${s} -s format png ${PNG_1024} --out ${ICONSET}/icon_${s}x${s}.png`.quiet()
  await $`sips -z ${s * 2} ${s * 2} -s format png ${PNG_1024} --out ${ICONSET}/icon_${s}x${s}@2x.png`.quiet()
}

// 4) iconutil 打包 icns
await $`iconutil -c icns ${ICONSET} -o ${OUT}`.quiet()
console.log(`图标产物: ${OUT}`)
