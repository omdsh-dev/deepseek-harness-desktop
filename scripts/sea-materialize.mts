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

/** 包的运行时依赖声明（name → semver range）；devDependencies 不参与运行时闭包。 */
function depRangesOf(pkg: Record<string, any> | null): Map<string, string> {
  const out = new Map<string, string>()
  for (const k of ['dependencies', 'peerDependencies', 'optionalDependencies']) {
    for (const [d, r] of Object.entries(pkg?.[k] ?? {})) out.set(d, r)
  }
  return out
}

/** 复制一个本地包（workspace/vendor）的运行时面：lib + dist + assets + package.json。
 *  bundle 包（package.json 的 dsh.bundle.patch）的 patch 文件（如 cordis.patch.yml）
 *  是 dsh 启动时 loadProfile 必读的运行时文件，一并复制。 */
function copyLocalPkg(name: string, srcDir: string): Record<string, any> | null {
  const dstDir = path.join(DST, ...name.split('/'))
  fs.mkdirSync(dstDir, { recursive: true })
  fs.copyFileSync(path.join(srcDir, 'package.json'), path.join(dstDir, 'package.json'))
  for (const d of ['lib', 'dist', 'assets']) {
    if (fs.existsSync(path.join(srcDir, d))) {
      fs.cpSync(path.join(srcDir, d), path.join(dstDir, d), { recursive: true })
    }
  }
  const pkg = pkgJsonOf(srcDir)
  const bundlePatch = pkg?.dsh?.bundle?.patch
  if (typeof bundlePatch === 'string' && bundlePatch) {
    const src = path.join(srcDir, bundlePatch)
    if (fs.existsSync(src)) {
      const dst = path.join(dstDir, bundlePatch)
      fs.mkdirSync(path.dirname(dst), { recursive: true })
      fs.copyFileSync(src, dst)
    }
  }
  return pkg
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
// 扁平布局（目标 node_modules 每包一份）：同一依赖名在闭包内只能落地一个版本，
// 复制顺序由依赖声明驱动——闭包内所有运行时消费者对同一包名声明兼容范围时，
// 首个满足者即全局一致解；冲突（多版本并存）时保留先复制的并告警。
// BFS 队列按父包声明的 semver range 从 .store 匹配版本，杜绝"取 .store 任意
// 第一个目录"导致的版本错配（如 ajv@6 顶替 peer 要求的 ajv@8）。
const done = new Set<string>()
const seen = new Map<string, string>() // name@version → 已复制的 srcDir
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

/** 把版本字符串解析成数字三元组（忽略 pre-release / build 元数据）。 */
function versionTuple(v: string): [number, number, number] {
  const [a, b, c] = v.split(/[-+]/)[0].split('.').map(n => parseInt(n, 10))
  return [a || 0, b || 0, c || 0]
}

/** 极简 semver 单段匹配：精确、部分版本（1 / 1.2 / 1.2.x）、^、~、比较符、*。
 *  无法解析的 range 保守放行（返回 true），避免闭包因未知语法整体失败。 */
function matchOne(version: string, r: string): boolean {
  r = r.trim()
  if (r === '*' || r === '') return true
  const m = r.match(/^(\^|~|>=|<=|>|<|=)?\s*v?(\d+|\*|x|X)(?:\.(\d+|\*|x|X))?(?:\.(\d+|\*|x|X))?$/)
  if (!m) return true
  const op = m[1] ?? '='
  const t = m.slice(2).map(s => (s == null || s === '*' || s === 'x' || s === 'X') ? null : Number(s))
  const v = versionTuple(version)
  const full = t[0] !== null && t[1] !== null && t[2] !== null
  const cmp = (n: number, tn: number | null): number => (tn === null ? 0 : n - tn)
  const c = cmp(v[0], t[0]) || cmp(v[1], t[1]) || cmp(v[2], t[2])
  switch (op) {
    case '=':
      if (full) return c === 0
      if (t[1] === null) return v[0] === t[0]
      if (t[2] === null) return v[0] === t[0] && v[1] === t[1]
      return v[0] === t[0] && v[1] === t[1] && v[2] === t[2]
    case '>': return c > 0
    case '<': return c < 0
    case '>=': return c >= 0
    case '<=': return c <= 0
    case '^': {
      if (t[0] === null) return true
      const vv = versionTuple(version)
      if (t[0] > 0) return c >= 0 && vv[0] < t[0] + 1
      if ((t[1] ?? 0) > 0) return c >= 0 && vv[1] < (t[1] ?? 0) + 1
      return c >= 0 && vv[2] < (t[2] ?? 0) + 1
    }
    case '~': {
      if (t[0] === null) return true
      const vv = versionTuple(version)
      if (t[1] === null) return c >= 0 && vv[0] < t[0] + 1
      return c >= 0 && vv[1] < t[1] + 1
    }
    default: return true
  }
}

/** 匹配 semver range：支持 || 与空白 AND。 */
function satisfies(version: string, range: string): boolean {
  for (const alt of range.split('||')) {
    const ands = alt.split(/\s+/).filter(Boolean)
    if (ands.every(r => matchOne(version, r))) return true
  }
  return false
}

// npm 依赖闭包：从 nub 的 .store 实体复制（版本感知）
let npmCopied = 0
while (queue.length) {
  const { name, pkg } = queue.shift()!
  for (const [dep, range] of depRangesOf(pkg)) {
    if (dep.startsWith('node:') || dep.startsWith('@types/')) continue
    if (dep.startsWith('workspace:') || done.has(dep)) continue
    const esc = dep.replace(/\//g, '+')
    // esc 含 '+'（包名 '/' 的转义），在正则里是量词，须转义。
    const escRe = esc.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    const matches = fs.globSync(path.join(STORE, `${esc}@*`))
      .map(dir => {
        const m = path.basename(dir).match(new RegExp(`^${escRe}@([^_]+)`))
        return { dir, version: m?.[1] ?? '' }
      })
      .filter(x => satisfies(x.version, range))
    if (!matches.length) {
      console.warn(`[warn] npm 依赖 ${dep}（需要 ${range}）在 .store 中无匹配版本，跳过`)
      continue
    }
    const hit = matches[0]
    const key = `${dep}@${hit.version}`
    if (seen.has(key)) continue
    const srcDir = path.join(hit.dir, 'node_modules', dep)
    if (!fs.existsSync(srcDir)) {
      console.warn(`[warn] npm 依赖 ${dep} 无实体，跳过`)
      continue
    }
    seen.set(key, srcDir)
    const dstDir = path.join(DST, ...dep.split('/'))
    const existing = fs.existsSync(dstDir) ? pkgJsonOf(dstDir) : null
    if (existing) {
      // 扁平布局同名冲突：版本不同则保留先复制的（首个满足者），版本相同则复用。
      if (existing.version !== hit.version) {
        console.warn(`[warn] 扁平布局下 ${dep} 多版本并存（已复制 ${existing.version}，跳过 ${hit.version}）`)
        continue
      }
    } else {
      fs.mkdirSync(path.dirname(dstDir), { recursive: true })
      fs.cpSync(srcDir, dstDir, { recursive: true })
      npmCopied++
    }
    queue.push({ name: dep, pkg: pkgJsonOf(srcDir) })
  }
}

console.log(`SEA 资源实体化完成: target/sea（config + package.json + ${npmCopied} 个 npm 包）`)
