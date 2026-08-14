// dsh SEA 薄入口（由 deepseek-harness-desktop CLI 生成）。
// blob 内只保留 node: builtin 导入；dsh CLI 与全部依赖经闭包内的
// dsh-bridge（CJS 桥，sea.Build 生成）从可执行文件旁的外部 node_modules
// 解析加载：桥的 import() 走正常 Node ESM loader，原生模块、顶层 await
// 与运行时资源均随闭包外置，无需 bundler 内联。
import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
require("dsh-bridge");
