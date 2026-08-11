/**
 * dsh SEA（Single Executable Application）打包入口。
 *
 * 在启动 dsh CLI 之前注册 tsx 的 ESM transform loader。dsh 的 cordis 插件机制
 * 在源码模式依赖 tsx 的 transform hook（workflow worker 的未打包分支、
 * dsh-sdk dev runtime、Windows directory-picker worker 等）；打包产物不内联
 * tsx（其 esbuild 原生依赖与 loader 文件机制无法内联），故 SEA 启动时从可执行
 * 文件旁的 node_modules（Contents/node_modules/tsx，已随 SEA 实体化）解析并
 * 注册 loader。
 *
 * 为什么用 CJS require 而不是动态 import / `--import` execArgv：
 * - Node SEA 的 embedder 动态 `import()` 只接受 builtin 模块，外部文件
 *   （含 file URL）一律 ERR_UNKNOWN_BUILTIN_MODULE；
 * - `--import` 的相对路径按 cwd 解析，而 dsh-server 的 cwd 是
 *   DSH_APP_WORKSPACE（默认用户主目录），`./node_modules/...` 解析不到会 fatal；
 * - CJS `require()` 是 SEA 的既定外部模块加载通道：createRequire(import.meta.url)
 *   把解析锚定在 SEA 可执行文件位置（Contents/MacOS），向上走 node_modules
 *   找到 Contents/node_modules/tsx，与 cwd 无关。tsx 的 esm/api CJS 变体内部
 *   用 __filename 定位 loader.mjs，require 加载时路径正确。
 */
import { createRequire } from 'node:module'

try {
  const require = createRequire(import.meta.url)
  const { register } = require('tsx/esm/api')
  register()
} catch (error) {
  // tsx 缺失/加载失败不阻断启动：dsh 核心路径（编译产物）不依赖 tsx，
  // 需要 tsx 的源码模式路径由壳注入的 NODE_OPTIONS 在子进程中提供。
  console.error(`[dsh] tsx ESM loader 注册失败（继续启动）: ${String(error)}`)
}

// dsh CLI 入口：静态依赖，随 SEA 内联；其顶层代码即 dsh 的 main。
// 从 npm 发布的 @deepseek-ai/dsh 包（node_modules）取 lib/bin.js，
// 不再从上游源码树（deepseek-harness/apps/cli/lib/types/）取构建产物。
await import('../node_modules/@deepseek-ai/dsh/lib/bin.js')
