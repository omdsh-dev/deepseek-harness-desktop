#!/usr/bin/env zx
/**
 * 组装 Windows 桌面应用 target/windows/DSH/：
 *   bin/dsh-server.exe  SEA 可执行（Windows 上由 `just sea` 产出 dsh.exe）
 *   bin/dsh-shell.exe   Wails 壳（justfile 中 go build 输出到该位置）
 *   config/ node_modules/ package.json   运行时资源（dsh-server 从 bin 上一级解析）
 *   dsh.ico             app 图标（多尺寸 PNG 内嵌的 ico，Vista+ 支持）
 * 组装后打包 target/windows/DSH.zip（顶层目录 DSH/）。
 *
 * 仅在 Windows 上执行：SEA（Node --build-sea）与 Wails 壳（WebView2）
 * 均不支持交叉编译，产物必须由 Windows 主机构建。本脚本不区分平台，在
 * macOS 上干跑会得到 macOS 的 dsh-server（仅用于验证组装逻辑）。
 *
 * 用法：`zx scripts/make-windows-app.mts`（前置：sea 已构建；壳由 justfile 输出）
 */
import { $, fs, path } from 'zx'
import sharp from 'sharp'

const ROOT = path.join(import.meta.dirname, '..')
const SEA = path.join(ROOT, 'target/sea')
const WIN = path.join(ROOT, 'target/windows')
const APP = path.join(WIN, 'DSH')
const ICON_SRC = path.join(ROOT, 'apps/dsh-desktop/assets/icon.svg')

// SEA 产物在 Windows 上是 dsh.exe，其他平台是 dsh（干跑验证用）。
const SEA_BIN = path.join(SEA, 'bin')
const seaExe = fs.existsSync(path.join(SEA_BIN, 'dsh.exe'))
  ? 'dsh.exe'
  : fs.existsSync(path.join(SEA_BIN, 'dsh'))
    ? 'dsh'
    : null
if (!seaExe) {
  console.error(`未找到 SEA 产物：${path.join(SEA_BIN, 'dsh[.exe]')}（先运行 just sea）`)
  process.exit(1)
}
// 后端文件名：Windows 上必须带 .exe（壳 exec.Command 不自动补扩展名）。
const serverExe = seaExe.endsWith('.exe') ? 'dsh-server.exe' : 'dsh-server'

fs.rmSync(WIN, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 })
fs.mkdirSync(path.join(APP, 'bin'), { recursive: true })

fs.copyFileSync(path.join(SEA_BIN, seaExe), path.join(APP, 'bin', serverExe))
fs.cpSync(path.join(SEA, 'config'), path.join(APP, 'config'), { recursive: true })
fs.cpSync(path.join(SEA, 'node_modules'), path.join(APP, 'node_modules'), { recursive: true })
fs.copyFileSync(path.join(SEA, 'package.json'), path.join(APP, 'package.json'))

// 图标：多尺寸 PNG 内嵌的 ico（ICONDIR + ICONDIRENTRY + PNG blobs，Vista+
// 支持 PNG 压缩条目）。sharp（libvips + librsvg）把 icon.svg 渲染为白底黑图，
// 与 make-linux-app.mts 同一渲染路径（resvg-js 无法解析该 SVG）。ico 无
// 外部工具依赖，纯 node 组装。
const ICON_SIZES = [16, 24, 32, 48, 64, 128, 256]
const svg = fs.readFileSync(ICON_SRC)
const big = await sharp(svg, { density: 1440 })
  .resize(1024, 1024, { fit: 'contain', background: { r: 255, g: 255, b: 255, alpha: 1 } })
  .png()
  .toBuffer()

function makeIco(pngs: Buffer[]): Buffer {
  const header = Buffer.alloc(6)
  header.writeUInt16LE(0, 0) // reserved
  header.writeUInt16LE(1, 2) // type: icon
  header.writeUInt16LE(pngs.length, 4)
  const entries: Buffer[] = []
  let offset = 6 + 16 * pngs.length
  for (let i = 0; i < pngs.length; i++) {
    const size = ICON_SIZES[i]!
    const e = Buffer.alloc(16)
    e.writeUInt8(size >= 256 ? 0 : size, 0) // width（256 用 0）
    e.writeUInt8(size >= 256 ? 0 : size, 1) // height（256 用 0）
    e.writeUInt8(0, 2) // palette
    e.writeUInt8(0, 3) // reserved
    e.writeUInt16LE(1, 4) // planes
    e.writeUInt16LE(32, 6) // bpp
    e.writeUInt32LE(pngs[i]!.length, 8)
    e.writeUInt32LE(offset, 12)
    offset += pngs[i]!.length
    entries.push(e)
  }
  return Buffer.concat([header, ...entries, ...pngs])
}

const pngs: Buffer[] = []
for (const size of ICON_SIZES) {
  pngs.push(await sharp(big).resize(size, size).png().toBuffer())
}
fs.writeFileSync(path.join(APP, 'dsh.ico'), makeIco(pngs))

// 打包 DSH.zip：用 bsdtar（macOS 与 Windows 10 1803+ 均自带，-a 按后缀
// 自动选 zip 格式）。缺失/失败时保留目录产物并告警，不阻断组装。
try {
  await $`tar -a -cf ${path.join(WIN, 'DSH.zip')} -C ${WIN} DSH`
} catch {
  console.warn('[warn] DSH.zip 打包失败（需要 bsdtar），已保留目录产物 target/windows/DSH')
}

console.log(`已生成：${APP}`)
console.log(`启动：${path.join(APP, 'bin', 'dsh-shell.exe')}`)
