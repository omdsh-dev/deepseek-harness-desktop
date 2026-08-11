import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "tsdown";

/**
 * dsh SEA（Single Executable Application）打包配置。
 * 以 scripts/sea-entry.ts 为入口：先注册 tsx ESM loader（SEA 旁 node_modules 的
 * tsx，见 sea-entry.ts），再内联 npm 发布的 @deepseek-ai/dsh 包
 * （node_modules/@deepseek-ai/dsh/lib/bin.js）及其全部 JS 依赖，用 Node 的
 * --build-sea 生成单文件可执行。原生模块与运行时资源（config、frontend dist、
 * 插件闭包 node_modules）外置在可执行文件旁，见 justfile 的 sea recipe。
 *
 * alias 的用途：npm 版 dsh 的 lib/bin.js（tsdown 打包产物）按包名 import
 * @deepseek-ai/* 与第三方依赖；这些包在 nub 布局下只存在于
 * node_modules/.store（顶层 node_modules 只有项目的直接依赖，无提升），
 * rolldown 从 scripts/sea-entry.ts 向上解析不到，会被误判为 external，
 * SEA 内联产物里保留裸包名 import，运行时即报 ERR_UNKNOWN_BUILTIN_MODULE。
 * 打包前 sea-materialize.mts 已把 dsh 依赖闭包扁平实体化到
 * target/sea/node_modules（版本由 BFS 解析确定），这里遍历该目录为每个包
 * 生成 alias 指向实体目录，让 rolldown 直接解析并内联，且内联版本与
 * 实体化闭包严格一致。
 */
const HERE = dirname(fileURLToPath(import.meta.url));

/** 遍历扁平 node_modules（顶层包 + @scope/* 两层），生成 包名 → 实体目录 的映射。 */
function collectPackages(root: string, out: Map<string, string>): void {
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue
    const sub = join(root, entry.name)
    if (entry.name.startsWith("@")) {
      collectPackages(sub, out)
      continue
    }
    const manifest = join(sub, "package.json")
    if (!existsSync(manifest)) continue
    try {
      const pkg = JSON.parse(readFileSync(manifest, "utf8"))
      if (typeof pkg.name === "string" && pkg.name !== "") out.set(pkg.name, sub)
    } catch {
      // 损坏的 package.json 跳过：对应包无法被 rolldown 解析时构建日志会暴露。
    }
  }
}

const alias = new Map<string, string>()
collectPackages(resolve(HERE, "target/sea/node_modules"), alias)

export default defineConfig({
  entry: ["scripts/sea-entry.ts"],
  format: ["esm"],
  platform: "node",
  target: "es2024",
  dts: false,
  clean: true,
  // 普通 bundle（无用途的中间产物）也收拢到 target/sea 下，避免污染根目录 dist/。
  outDir: "target/sea/dist",
  deps: { alwaysBundle: /./ },
  // 闭包内全部包解析到 target/sea/node_modules 实体（见文件头注释）。
  alias: Object.fromEntries(alias),
  exe: {
    fileName: "dsh",
    // 可执行文件必须落在深层目录：代码里 `new URL('../config/…', import.meta.url)`
    // 从 exe 所在目录的上一级解析运行时资源（见 apps/cli/src/web.ts 等）。
    outDir: "target/sea/bin",
    seaConfig: {
      execArgv: ["--expose-internals"],
    },
  },
});
