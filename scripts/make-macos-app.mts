#!/usr/bin/env zx
/**
 * 组装 macOS 桌面应用 target/DSH.app：拷贝 SEA 产物与图标，写 Info.plist。
 * Wails 壳（dsh-shell）由 justfile 在组装后用 `go build` 直接输出到
 * Contents/MacOS/，本脚本不负责壳。
 * 布局：
 *   Contents/MacOS/dsh-shell   Wails 壳（Go，由 go build 产出）
 *   Contents/MacOS/dsh-server  SEA 可执行（内嵌 node）
 *   Contents/Resources/dsh.icns  app 图标
 *   Contents/{config,node_modules,package.json}  资源（dsh-server 从上一级解析）
 *
 * 用法：`zx scripts/make-dsh-app.mts`（前置：sea + icon 已构建）
 */
import { fs, path } from 'zx'

const ROOT = path.join(import.meta.dirname, '..')
const SEA = path.join(ROOT, 'target/sea')
const APP = path.join(ROOT, 'target/DSH.app')
const ICON = path.join(ROOT, 'target/dsh.icns')
const APP_NAME = 'dsh-shell'

if (!fs.existsSync(path.join(SEA, 'bin/dsh'))) {
  console.error(`未找到 SEA 产物：${path.join(SEA, 'bin/dsh')}（先运行 just sea）`)
  process.exit(1)
}

fs.rmSync(APP, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 })
fs.mkdirSync(path.join(APP, 'Contents/MacOS'), { recursive: true })
fs.mkdirSync(path.join(APP, 'Contents/Resources'), { recursive: true })

fs.copyFileSync(path.join(SEA, 'bin/dsh'), path.join(APP, 'Contents/MacOS/dsh-server'))
fs.cpSync(path.join(SEA, 'config'), path.join(APP, 'Contents/config'), { recursive: true })
fs.cpSync(path.join(SEA, 'node_modules'), path.join(APP, 'Contents/node_modules'), { recursive: true })
fs.copyFileSync(path.join(SEA, 'package.json'), path.join(APP, 'Contents/package.json'))

fs.chmodSync(path.join(APP, 'Contents/MacOS/dsh-server'), 0o755)

// app 图标（icon recipe 产出；缺失时跳过，不阻断组装）
if (fs.existsSync(ICON)) {
  fs.copyFileSync(ICON, path.join(APP, 'Contents/Resources/dsh.icns'))
}

const iconKey = fs.existsSync(ICON)
  ? `        <key>CFBundleIconFile</key>
            <string>dsh</string>
`
  : ''

const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
    <dict>
        <key>CFBundlePackageType</key>
            <string>APPL</string>
        <key>CFBundleName</key>
            <string>dsh</string>
        <key>CFBundleDisplayName</key>
            <string>dsh — DeepSeek Harness</string>
        <key>CFBundleExecutable</key>
            <string>${APP_NAME}</string>
${iconKey}        <key>CFBundleIdentifier</key>
            <string>ai.deepseek.dsh</string>
        <key>CFBundleVersion</key>
            <string>0.0.1</string>
        <key>CFBundleShortVersionString</key>
            <string>0.0.1</string>
        <key>LSMinimumSystemVersion</key>
            <string>12.0.0</string>
        <key>NSHighResolutionCapable</key>
            <string>true</string>
    </dict>
</plist>
`
fs.writeFileSync(path.join(APP, 'Contents/Info.plist'), infoPlist)

console.log(`已生成：${APP}`)
console.log(`启动验证：${path.join(APP, 'Contents/MacOS/dsh-shell')}（需图形会话）`)
console.log(`运行：open ${APP}`)
