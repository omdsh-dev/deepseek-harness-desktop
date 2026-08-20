// dsh SEA 打包配置（由 dsh-web-desktopify CLI 生成）。
// 薄入口：只打包 sea-entry.mjs 自身（node: builtin 导入），dsh CLI 与
// 依赖树一律不内联——运行时经闭包内 dsh-bridge 从可执行文件旁的
// node_modules（扁平闭包）走正常 Node 解析。闭包即本目录 node_modules，
// createRequire 从 exe 路径向上解析。
export default {
  entry: ['sea-entry.mjs'],
  format: ['esm'],
  platform: 'node',
  target: 'es2024',
  dts: false,
  clean: true,
  // 普通 bundle（无用途的中间产物）也收拢到 dist/ 下，避免污染目录根。
  outDir: 'dist',
  // 不内联任何依赖（匹配空串的正则）：全部保持外部解析。原生模块
  // （.node）无法内联，bundler 内联反而会留下解析不到的裸导入。
  deps: { onlyBundle: /^$/ },
  exe: {
    fileName: 'dsh',
    // 可执行文件必须落在深层目录：createRequire(import.meta.url) 以 exe
    // 所在目录为解析起点向上找 node_modules 闭包。
    outDir: 'bin',
    seaConfig: {
      execArgv: ['--expose-internals'],
    },
  },
}
