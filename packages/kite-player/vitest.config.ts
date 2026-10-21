import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "happy-dom",
    include: ["test/**/*.test.ts"],
    // vitest 默认把 CSS 换成空字符串;元素的样式经 ?raw 导入,需要真实内容。
    css: { include: [/src\/element\/styles\/.+\.css/] },
  },
});
