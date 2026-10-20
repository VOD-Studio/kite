import { defineConfig } from "tsup";

export default defineConfig([
  {
    // npm 用户:ESM + 类型声明,react 由使用方提供。
    entry: { index: "src/index.ts", element: "src/element.ts", react: "src/react.ts" },
    format: ["esm"],
    dts: true,
    sourcemap: true,
    clean: true,
    target: "es2022",
    external: ["react"],
  },
  {
    // 静态博客:<script src> 一个文件即可,自动注册 <kite-player> 等元素。
    entry: { "kite-player": "src/element.ts" },
    format: ["iife"],
    globalName: "KitePlayer",
    minify: true,
    sourcemap: true,
    target: "es2020",
    outExtension: () => ({ js: ".iife.js" }),
  },
]);
