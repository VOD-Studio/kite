import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { defineConfig, type Options } from "tsup";

type EsbuildPlugin = NonNullable<Options["esbuildPlugins"]>[number];

/**
 * rawCss:`import css from "./x.css?raw"` 得到压缩后的 CSS 字符串。
 * 元素的样式在 Shadow DOM 里,必须随 JS 一起分发;源码仍按普通 .css 文件维护。
 */
const rawCss: EsbuildPlugin = {
  name: "raw-css",
  setup(build) {
    // 解析后的路径保留 ?raw 后缀:tsup 自带的 postcss 插件按 /\.css$/ 拦截所有命名空间的加载。
    build.onResolve({ filter: /\.css\?raw$/ }, (args) => ({
      path: resolve(args.resolveDir, args.path),
      namespace: "raw-css",
    }));
    build.onLoad({ filter: /.*/, namespace: "raw-css" }, async (args) => {
      const file = args.path.replace(/\?raw$/, "");
      const { code } = await build.esbuild.transform(await readFile(file, "utf8"), { loader: "css", minify: true });
      return { contents: code.trim(), loader: "text", watchFiles: [file] };
    });
  },
};

export default defineConfig([
  {
    // npm 用户:ESM + 类型声明,react 由使用方提供。
    entry: { index: "src/index.ts", element: "src/element.ts", react: "src/react/index.ts" },
    format: ["esm"],
    dts: true,
    sourcemap: true,
    clean: true,
    target: "es2022",
    external: ["react"],
    esbuildPlugins: [rawCss],
  },
  {
    // 静态页面:<script src> 一个文件即可,自动注册 <kite-player> 等元素。
    entry: { "kite-player": "src/element.ts" },
    format: ["iife"],
    globalName: "KitePlayer",
    minify: true,
    sourcemap: true,
    target: "es2020",
    outExtension: () => ({ js: ".iife.js" }),
    esbuildPlugins: [rawCss],
  },
]);
