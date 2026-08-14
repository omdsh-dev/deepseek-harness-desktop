// dsh SEA 打包配置（由 deepseek-harness-desktop CLI 生成）。
// 内联 entry 及其静态依赖，用 Node 的 --build-sea 生成单文件可执行。
// 原生模块与运行时资源（config、frontend dist、插件闭包 node_modules）
// 外置在可执行文件旁。闭包即本目录 node_modules（扁平布局），bundler
// 直接从本地解析，无需 alias 映射。
export default {
  entry: ['sea-entry.mjs'],
  format: ['esm'],
  platform: 'node',
  target: 'es2024',
  dts: false,
  clean: true,
  // 普通 bundle（无用途的中间产物）也收拢到 dist/ 下，避免污染目录根。
  outDir: 'dist',
  deps: { alwaysBundle: /./ },
  exe: {
    fileName: 'dsh',
    // 可执行文件必须落在深层目录：代码里 new URL('../config/…', import.meta.url)
    // 从 exe 所在目录的上一级解析运行时资源。
    outDir: 'bin',
    seaConfig: {
      execArgv: ['--expose-internals'],
    },
  },
}
