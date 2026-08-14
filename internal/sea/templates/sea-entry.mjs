// dsh SEA 打包入口（由 deepseek-harness-desktop CLI 生成）。
// 直接启动 dsh CLI：lib/bin.js 及其静态依赖在构建期由 bundler 内联进
// SEA blob。不集成 tsx —— 正式打包产物不启用 dsh 的源码模式路径（需要
// tsx transform hook 的未打包分支）。
await import('./node_modules/@deepseek-ai/dsh/lib/bin.js')
