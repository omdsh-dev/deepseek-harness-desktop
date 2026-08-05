#!/usr/bin/env zx
/**
 * dsh SEA 运行时资源实体化。
 *
 * tsdown 的 exe 功能把 cli 入口及其静态依赖内联进单文件可执行（target/sea/bin/dsh），
 * 但 dsh 的两类运行时依赖无法内联，必须外置在可执行文件旁：
 *
 * 1. 资源文件：`../config/*.cordis.yml`（cordis 配置树）与 `../package.json`
 *    （版本号），代码通过 `new URL('../…', import.meta.url)` 从 exe 上一级解析。
 * 2. 插件包：base.cordis.yml 声明的上百个 cordis 插件由 loader 在运行时以
 *    字符串包名 `import()`（vendor/loader/src/config/tree.ts），无法静态内联，
 *    需要可解析的 node_modules（workspace/vendor 包的 lib 产物 + npm 依赖闭包）。
 *
 * 用法：`zx scripts/sea-materialize.mts`（在 `just sea` 中由 tsdown 打包后执行）。
 */
import { $, fs, path } from 'zx'

const ROOT = path.join(import.meta.dirname, '..')
const CLI = path.join(ROOT, 'deepseek-harness/apps/cli')
const STORE = path.join(ROOT, 'node_modules/.store')
const SEA = path.join(ROOT, 'target/sea')
const DST = path.join(SEA, 'node_modules')

function pkgJsonOf(dir: string): Record<string, any> | null {
  try {
    return JSON.parse(fs.readFileSync(path.join(dir, 'package.json'), 'utf8'))
  } catch {
    return null
  }
}

function depsOf(pkg: Record<string, any> | null): string[] {
  const out: string[] = []
  for (const k of ['dependencies', 'peerDependencies', 'optionalDependencies']) {
    for (const d of Object.keys(pkg?.[k] ?? {})) out.push(d)
  }
  return out
}

/** 复制一个本地包（workspace/vendor）的运行时面：lib + dist + assets + package.json。 */
function copyLocalPkg(name: string, srcDir: string): Record<string, any> | null {
  const dstDir = path.join(DST, ...name.split('/'))
  fs.mkdirSync(dstDir, { recursive: true })
  fs.copyFileSync(path.join(srcDir, 'package.json'), path.join(dstDir, 'package.json'))
  for (const d of ['lib', 'dist', 'assets']) {
    if (fs.existsSync(path.join(srcDir, d))) {
      fs.cpSync(path.join(srcDir, d), path.join(dstDir, d), { recursive: true })
    }
  }
  return pkgJsonOf(srcDir)
}

// ── 1) 资源文件：config 与 package.json ──────────────────────────────────────
// 保留 tsdown 刚生成的 exe（target/sea/bin/），只重建外置资源。
fs.rmSync(DST, { recursive: true, force: true })
fs.rmSync(path.join(SEA, 'config'), { recursive: true, force: true })
fs.rmSync(path.join(SEA, 'package.json'), { force: true })
fs.mkdirSync(DST, { recursive: true })
fs.cpSync(path.join(CLI, 'config'), path.join(SEA, 'config'), { recursive: true })
fs.copyFileSync(path.join(CLI, 'package.json'), path.join(SEA, 'package.json'))
fs.mkdirSync(DST, { recursive: true })

// ── 2) node_modules 实体化 ───────────────────────────────────────────────────
const done = new Set<string>()
const seen = new Set<string>()
const queue: { name: string; pkg: Record<string, any> | null }[] = []

// workspace 包（packages/*/*）
for (const dir of fs.globSync(path.join(ROOT, 'deepseek-harness/packages', '*', '*'))) {
  const pkg = pkgJsonOf(dir)
  if (!pkg?.name?.startsWith('@deepseek-ai/')) continue
  copyLocalPkg(pkg.name, dir)
  done.add(pkg.name)
  queue.push({ name: pkg.name, pkg })
}

// vendor 包（cordis、cosmokit、@cordisjs/*、schemastery 等）
for (const dir of fs.globSync(path.join(ROOT, 'deepseek-harness/vendor', '*'))) {
  const pkg = pkgJsonOf(dir)
  if (!pkg?.name) continue
  copyLocalPkg(pkg.name, dir)
  done.add(pkg.name)
  queue.push({ name: pkg.name, pkg })
}

// dsh-frontend（apps/web，含浏览器端 dist 构建产物）
{
  const name = '@deepseek-ai/dsh-frontend'
  const pkg = copyLocalPkg(name, path.join(ROOT, 'deepseek-harness/apps/web'))
  done.add(name)
  queue.push({ name, pkg })
}

// npm 依赖闭包：从 nub 的 .store 实体复制
let npmCopied = 0
while (queue.length) {
  const { name, pkg } = queue.shift()!
  for (const dep of depsOf(pkg)) {
    if (dep.startsWith('node:') || dep.startsWith('@types/')) continue
    if (done.has(dep) || seen.has(dep)) continue
    seen.add(dep)
    const esc = dep.replace(/\//g, '+')
    const matches = fs.globSync(path.join(STORE, `${esc}@*`))
    if (!matches.length) {
      console.warn(`[warn] npm 依赖 ${dep} 不在 .store 中，跳过`)
      continue
    }
    const srcDir = path.join(matches[0], 'node_modules', dep)
    if (!fs.existsSync(srcDir)) {
      console.warn(`[warn] npm 依赖 ${dep} 无实体，跳过`)
      continue
    }
    const dstDir = path.join(DST, ...dep.split('/'))
    fs.mkdirSync(path.dirname(dstDir), { recursive: true })
    fs.cpSync(srcDir, dstDir, { recursive: true })
    npmCopied++
    queue.push({ name: dep, pkg: pkgJsonOf(srcDir) })
  }
}

console.log(`SEA 资源实体化完成: target/sea（config + package.json + ${npmCopied} 个 npm 包）`)
