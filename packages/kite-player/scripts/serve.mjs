// 起离线演示页看效果:npm run demo(先构建)或仓库根的 make player-demo(DEMO_HOST / DEMO_PORT)。
// 环境变量 HOST(默认 127.0.0.1)、PORT(默认 5174)。HOST=0.0.0.0 时同一局域网的手机也能打开,
// 演示页宽度不超过 767px 时是手机布局(全屏播放器、底部抽屉)。
import { networkInterfaces } from "node:os";
import { serveStatic } from "./static.mjs";

const host = process.env.HOST || "127.0.0.1";
const port = Number(process.env.PORT || 5174);
await serveStatic(host, port);

const path = "/demo/index.html";
console.log(`kite-player 演示页:http://127.0.0.1:${port}${path}`);
if (host === "0.0.0.0") {
  for (const list of Object.values(networkInterfaces())) {
    for (const a of list ?? []) {
      if (a.family === "IPv4" && !a.internal) console.log(`  局域网(手机打开):http://${a.address}:${port}${path}`);
    }
  }
} else {
  console.log("  手机上看:make player-demo DEMO_HOST=0.0.0.0(或 HOST=0.0.0.0 npm run demo),用同一局域网的手机打开打印出的地址");
}
console.log("改了代码后重新运行(会重新构建),页面不缓存。Ctrl+C 退出。");
