// `xxx.css?raw` 导入为字符串:构建时由 tsup.config.ts 的 rawCss 处理,测试时由 Vite 内置的 ?raw 处理。
declare module "*.css?raw" {
  const css: string;
  export default css;
}
