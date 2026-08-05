import { defineConfig } from "tsdown";

/**
 * dsh SEA（Single Executable Application）打包配置。
 * 以 cli 的 tsc 编译产物为入口，内联全部 JS 依赖（workspace 包 + npm 包），
 * 用 Node 的 --build-sea 生成单文件可执行。原生模块与运行时资源（config、
 * frontend dist）外置在可执行文件旁，见 justfile 的 sea recipe。
 */
export default defineConfig({
  entry: ["deepseek-harness/apps/cli/lib/types/bin.js"],
  format: ["esm"],
  platform: "node",
  target: "es2024",
  dts: false,
  clean: true,
  // 普通 bundle（无用途的中间产物）也收拢到 target/sea 下，避免污染根目录 dist/。
  outDir: "target/sea/dist",
  deps: { alwaysBundle: /./ },
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
