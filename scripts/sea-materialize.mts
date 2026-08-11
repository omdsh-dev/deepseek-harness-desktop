#!/usr/bin/env zx
/**
 * dsh SEA 运行时资源实体化。
 *
 * tsdown 的 exe 功能把 dsh 入口（node_modules/@deepseek-ai/dsh/lib/bin.js）及其
 * 静态依赖内联进单文件可执行（target/sea/bin/dsh），但 dsh 的两类运行时依赖
 * 无法内联，必须外置在可执行文件旁：
 *
 * 1. 资源文件：`../config/*.cordis.yml`（cordis 配置树）与 `../package.json`
 *    （版本号），代码通过 `new URL('../…', import.meta.url)` 从 exe 上一级解析。
 * 2. 插件包：cordis 插件由 loader 在运行时以字符串包名 `import()`
 *    （vendor/loader/src/config/tree.ts），无法静态内联，需要可解析的
 *    node_modules（@deepseek-ai/dsh 的 npm 依赖闭包实体）。
 *
 * 来源是 npm 发布的 @deepseek-ai/dsh（nub 安装于仓库根 node_modules）：
 * config/ 与 package.json 取自该包；插件闭包从其依赖声明出发，按 semver range
 * 从本仓库 node_modules/.store 匹配版本复制（npm 包实体即发布 files 内容，
 * 含 lib/dist/assets/bin 与 bundle patch 文件，如 cordis.patch.yml）。
 *
 * 用法：`zx scripts/sea-materialize.mts`（在 `just sea` 中先于 tsdown 执行：
 * tsdown.sea.config.ts 的 alias 依赖本脚本产出的 target/sea/node_modules）。
 */
import { $, fs, path } from 'zx'

const ROOT = path.join(import.meta.dirname, '..')
// npm 发布的 dsh 主包（nub 安装在仓库根 node_modules，见 package.json）。
const CLI = path.join(ROOT, 'node_modules/@deepseek-ai/dsh')
// npm 依赖闭包从本仓库的 .store 实体复制：nub 布局，dsh 的全部运行时依赖
// 安装在 node_modules/.store（顶层 node_modules 只有项目的直接依赖）。
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

// ── 1) 资源文件：config 与 package.json ──────────────────────────────────────
// 不触碰 tsdown 的 exe 产物（target/sea/bin/），只重建外置资源。
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

// 闭包入口：npm 安装的 @deepseek-ai/dsh 主包（其 dependencies 覆盖全部
// 运行时插件包；旧版的 workspace/vendor/dsh-frontend 包现均以 npm 依赖
// 形式存在于闭包内，由 BFS 统一复制）。
{
  const pkg = pkgJsonOf(CLI)
  if (!pkg?.name) {
    console.error(`未找到 npm 包 ${CLI}（先运行 just dep / nub install）`)
    process.exit(1)
  }
  done.add(pkg.name)
  queue.push({ name: pkg.name, pkg })
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

// npm 依赖闭包：从 nub 的 .store 实体复制（版本感知）。
// BFS 以 seed 队列为根展开依赖闭包；tsx 补充等后续追加的根复用同一复制逻辑。
let npmCopied = 0
function copyClosure(seed: { name: string; pkg: Record<string, any> | null }[]): void {
  const q = [...seed]
  while (q.length) {
  const { name, pkg } = q.shift()!
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
    q.push({ name: dep, pkg: pkgJsonOf(srcDir) })
  }
  }
}

// 闭包展开（dsh 主包为根）。
copyClosure(queue)

// tsx：SEA 启动时 sea-entry.ts 从 exe 旁的 node_modules 注册 tsx ESM loader
// （dsh 的源码模式路径需要）。tsx 不是 dsh 的依赖（桌面工具链 devDependency），
// BFS 不会复制；闭包若缺它则从 .store 显式补上，并复用 copyClosure 复制其
// 依赖（esbuild 等，tsx loader 运行必需）。
if (!fs.existsSync(path.join(DST, 'tsx'))) {
  const tsxDirs = fs.globSync(path.join(STORE, 'tsx@*'))
    .filter(dir => fs.existsSync(path.join(dir, 'node_modules/tsx')))
  if (tsxDirs.length) {
    const tsxDir = path.join(tsxDirs[0], 'node_modules/tsx')
    fs.cpSync(tsxDir, path.join(DST, 'tsx'), { recursive: true })
    copyClosure([{ name: 'tsx', pkg: pkgJsonOf(tsxDir) }])
    console.log('SEA 资源实体化: 补充 tsx（dsh 源码模式 loader）及其依赖')
  } else {
    console.warn('[warn] .store 中无 tsx 实体，SEA 的 tsx loader 注册将失败（dsh 编译产物路径不受影响）')
  }
}

console.log(`SEA 资源实体化完成: target/sea（config + package.json + ${npmCopied} 个 npm 包）`)
