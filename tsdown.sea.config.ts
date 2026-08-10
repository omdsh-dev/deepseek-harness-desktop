import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "tsdown";

/**
 * dsh SEA（Single Executable Application）打包配置。
 * 以 scripts/sea-entry.ts 为入口：先注册 tsx ESM loader（SEA 旁 node_modules 的
 * tsx，见 sea-entry.ts），再内联 cli 的 tsc 编译产物及其全部 JS 依赖（workspace
 * 包 + npm 包），用 Node 的 --build-sea 生成单文件可执行。原生模块与运行时资源
 * （config、frontend dist）外置在可执行文件旁，见 justfile 的 sea recipe。
 *
 * alias 的用途：workspace 包通过 package.json 的 dependencies 互相链接，
 * 但 peerDependencies 不会随 nub 布局提升到消费者的 node_modules。cli 的
 * lib 产物（tsc 编译）直接 import 这类 peer 包时，rolldown 从
 * apps/cli/lib/types/ 向上解析 node_modules 会找不到，被误判为 external，
 * SEA 内联产物里保留裸包名 import，运行时即报 ERR_UNKNOWN_BUILTIN_MODULE。
 * 这里把这类 peer 包 alias 到其 lib 产物目录，让 rolldown 直接解析并内联。
 */
const HERE = dirname(fileURLToPath(import.meta.url));

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
  // peer 依赖（cli 未在 dependencies 声明、nub 不提升）解析到其 lib 产物目录。
  alias: {
    "@deepseek-ai/dsh-environment": resolve(HERE, "deepseek-harness/packages/util/environment/lib"),
  },
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
