// <kite-player> 的样式。尺寸、颜色与动效参数来自设计稿(docs/player.md 链接的画布)。
// 站点只需覆盖 --kite-* 变量,或用 ::part() 微调;内部类名不是公开接口。

export const playerStyles = /* css */ `
:host {
  --kite-accent: #D33A40;
  --kite-font: -apple-system, BlinkMacSystemFont, "PingFang SC", "Hiragino Sans GB", "Noto Sans SC", "Microsoft YaHei", system-ui, sans-serif;
  --kite-offset: 24px;
  --kite-glass: linear-gradient(180deg, rgba(255,255,255,.66), rgba(255,255,255,.5));
  --kite-glass-edge: rgba(255,255,255,.95);
  --kite-glass-rim: rgba(255,255,255,.6);
  --kite-fg: #15171C;
  --kite-muted: #4A505C;
  --kite-line: rgba(21,23,28,.12);
  --kite-chip: rgba(21,23,28,.06);
  --kite-chip-strong: rgba(21,23,28,.12);
  --kite-sheet: #FFFFFF;
  --kite-ring-track: rgba(255,255,255,.96);
  --kite-blob: #15171C;
  --kite-blob-icon: #FFFFFF;
  --ease: cubic-bezier(.2,.85,.25,1);
  --ease-in: cubic-bezier(.5,0,.25,1);
  --spring: cubic-bezier(.34,1.3,.5,1);
  --lyric: cubic-bezier(.22,1,.36,1);
  font-family: var(--kite-font);
  -webkit-font-smoothing: antialiased;
}
:host([theme="dark"]) { --kite-glass: linear-gradient(180deg, rgba(44,46,54,.62), rgba(26,28,34,.5)); --kite-glass-edge: rgba(255,255,255,.2); --kite-glass-rim: rgba(255,255,255,.08); --kite-fg: #ECEDEF; --kite-muted: #A9AEB9; --kite-line: rgba(255,255,255,.14); --kite-chip: rgba(255,255,255,.08); --kite-chip-strong: rgba(255,255,255,.14); --kite-sheet: #1C1E24; --kite-ring-track: rgba(255,255,255,.22); --kite-blob: #F2F3F5; --kite-blob-icon: #15171C; }
@media (prefers-color-scheme: dark) {
  :host(:not([theme="light"])) { --kite-glass: linear-gradient(180deg, rgba(44,46,54,.62), rgba(26,28,34,.5)); --kite-glass-edge: rgba(255,255,255,.2); --kite-glass-rim: rgba(255,255,255,.08); --kite-fg: #ECEDEF; --kite-muted: #A9AEB9; --kite-line: rgba(255,255,255,.14); --kite-chip: rgba(255,255,255,.08); --kite-chip-strong: rgba(255,255,255,.14); --kite-sheet: #1C1E24; --kite-ring-track: rgba(255,255,255,.22); --kite-blob: #F2F3F5; --kite-blob-icon: #15171C; }
}
* { box-sizing: border-box; }
button { font: inherit; color: inherit; background: none; border: 0; padding: 0; margin: 0; cursor: pointer; -webkit-tap-highlight-color: transparent; }
button:focus-visible, input:focus-visible { outline: 2px solid var(--kite-accent); outline-offset: 2px; }
img { display: block; width: 100%; height: 100%; object-fit: cover; }
[hidden] { display: none !important; }
.ell { display: block; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

.ib { display: inline-grid; place-items: center; width: 40px; height: 40px; border-radius: 20px; flex: none; transition: background-color .16s, transform .16s var(--ease); }
.ib svg { width: 22px; height: 22px; }
.ib:hover { background: var(--kite-chip); }
.ib:active { transform: scale(.9); }
.blob { position: relative; display: grid; place-items: center; flex: none; transition: transform .16s var(--ease); }
.blob:active { transform: scale(.92); }
.blob svg { position: absolute; inset: 0; width: 100%; height: 100%; }
.blob .blob-shape { fill: var(--kite-blob); }
.blob .i-play, .blob .i-pause { fill: var(--kite-blob-icon); transform-origin: 32px 30px; transition: opacity .2s, transform .42s var(--spring); }
.blob .i-pause { opacity: 0; transform: scale(.4) rotate(-90deg); }
.playing .blob .i-play { opacity: 0; transform: scale(.4) rotate(90deg); }
.playing .blob .i-pause { opacity: 1; transform: none; }

.eq { display: inline-flex; align-items: flex-end; gap: 2px; height: 12px; }
.eq i { display: block; width: 2.5px; height: 100%; border-radius: 2px; background: var(--kite-accent); transform-origin: 50% 100%; transform: scaleY(.35); animation: eq 900ms ease-in-out infinite alternate; animation-play-state: paused; }
.eq i:nth-child(2) { animation-duration: 700ms; animation-delay: -300ms; }
.eq i:nth-child(3) { animation-duration: 1000ms; animation-delay: -600ms; }
.playing .eq i { animation-play-state: running; }
@keyframes eq { from { transform: scaleY(.25); } to { transform: scaleY(1); } }

/* ---------- 迷你播放条 / 圆形 ---------- */
.bar {
  position: fixed; z-index: 2147482000; right: var(--kite-offset); bottom: var(--kite-offset);
  width: 424px; height: 72px; border-radius: 36px; overflow: hidden; color: var(--kite-fg);
  background: var(--kite-glass);
  -webkit-backdrop-filter: blur(28px) saturate(1.9) brightness(1.05); backdrop-filter: blur(28px) saturate(1.9) brightness(1.05);
  box-shadow: inset 0 1px 0 var(--kite-glass-edge), inset 0 0 0 1px var(--kite-glass-rim), 0 0 0 .5px rgba(21,23,28,.12), 0 20px 46px -18px rgba(21,23,28,.34), 0 2px 8px rgba(21,23,28,.06);
  transition: width .48s var(--spring), height .42s var(--ease), border-radius .42s var(--ease), opacity .2s, transform .2s, visibility 0s;
}
.bar::before { content: ""; position: absolute; inset: 0; border-radius: inherit; background: radial-gradient(120% 160% at 10% -30%, rgba(255,255,255,.55), rgba(255,255,255,0) 55%); pointer-events: none; }
@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) { .bar { background: var(--kite-sheet); } }
.disc { position: absolute; left: 6px; top: 6px; width: 60px; height: 60px; border-radius: 50%; transition: left .42s var(--ease), top .42s var(--ease), width .42s var(--ease), height .42s var(--ease); }
.ring { position: absolute; inset: 0; border-radius: 50%; background: conic-gradient(var(--kite-accent) calc(var(--pct, 0) * 1turn), var(--kite-ring-track) 0); -webkit-mask: radial-gradient(farthest-side, transparent calc(100% - 3px), #000 calc(100% - 2.5px)); mask: radial-gradient(farthest-side, transparent calc(100% - 3px), #000 calc(100% - 2.5px)); }
.disc-img { position: absolute; inset: 3px; width: auto; height: auto; border-radius: 50%; overflow: hidden; background: #D9DCE2; }
.disc-img img { animation: spin 9s linear infinite; animation-play-state: paused; }
.playing .disc-img img { animation-play-state: running; }
@keyframes spin { to { transform: rotate(1turn); } }
.bar-text { position: absolute; left: 76px; right: 196px; top: 13px; height: 46px; text-align: left; border-radius: 10px; padding: 0 4px; transition: opacity .2s; }
.bar-title { font-size: 15px; line-height: 22px; font-weight: 600; }
.bar-sub { display: block; height: 20px; overflow: hidden; font-size: 13px; line-height: 20px; color: var(--kite-muted); }
.bar-sub .ell { animation: tick .5s var(--ease); }
@keyframes tick { from { transform: translateY(100%); opacity: 0; } to { transform: none; opacity: 1; } }
.bar-ctl { position: absolute; right: 8px; top: 14px; display: flex; align-items: center; gap: 4px; transition: opacity .2s; }
.bar-ctl .ib { width: 36px; height: 36px; }
.bar-ctl .ib svg { width: 20px; height: 20px; }
.bar-ctl .blob { width: 44px; height: 44px; }
.bar-sep { width: 1px; height: 24px; margin: 0 4px; background: var(--kite-line); }
.kp[data-mode="orb"] .bar { width: 64px; height: 64px; border-radius: 32px; }
.kp[data-mode="orb"] .disc { left: 2px; top: 2px; }
.kp[data-mode="orb"] .bar-text, .kp[data-mode="orb"] .bar-ctl { opacity: 0; pointer-events: none; }
.kp[data-mode="open"] .bar { opacity: 0; transform: scale(.98); pointer-events: none; visibility: hidden; transition: opacity .1s, transform .1s, visibility 0s .1s; }
.kp:not([data-has-track]) .bar { display: none; }
.kp[data-device="mobile"] .bar { right: 12px; bottom: calc(16px + env(safe-area-inset-bottom, 0px)); width: calc(100vw - 24px); height: 64px; border-radius: 32px; }
.kp[data-device="mobile"] .disc { left: 5px; top: 5px; width: 54px; height: 54px; }
.kp[data-device="mobile"] .bar-text { left: 66px; right: 132px; top: 9px; }
.kp[data-device="mobile"] .bar-ctl { right: 10px; top: 12px; }
.kp[data-device="mobile"] .bar-ctl .blob { width: 36px; height: 36px; }
.kp[data-device="mobile"] .bar-ctl .ib { width: 40px; height: 40px; }
.kp[data-device="mobile"] .desk { display: none; }
.kp[data-device="mobile"][data-mode="orb"] .bar { width: 56px; height: 56px; border-radius: 28px; }
.kp[data-device="mobile"][data-mode="orb"] .disc { left: 1px; top: 1px; }

/* ---------- 展开:桌面面板 / 手机全屏 / 沉浸 ---------- */
.stage {
  position: fixed; z-index: 2147482500; right: var(--kite-offset); bottom: var(--kite-offset);
  width: 400px; height: min(712px, calc(100vh - 2 * var(--kite-offset)));
  border-radius: 28px; overflow: hidden; color: #FFFFFF; background: #2B3350;
  box-shadow: 0 40px 80px -30px rgba(10,12,30,.6), inset 0 0 0 1px rgba(255,255,255,.08);
  --pad: 32px; --h: min(712px, calc(100vh - 2 * var(--kite-offset)));
  --cs: clamp(160px, calc(var(--h) - 376px), 336px); --cx: calc((400px - var(--cs)) / 2); --cy: 60px;
  --ss: 48px; --sx: 28px; --sy: 62px; --ly-top: 126px; --ctl-h: 176px; --safe-b: 0px; --safe-t: 0px;
}
.kp[data-device="mobile"] .stage {
  inset: 0; width: auto; height: auto; border-radius: 0; box-shadow: none;
  --pad: 24px; --safe-t: env(safe-area-inset-top, 0px); --safe-b: env(safe-area-inset-bottom, 0px);
  --cs: max(160px, min(calc(100vw - 48px), calc(100vh - 470px))); --cx: calc((100vw - var(--cs)) / 2); --cy: calc(var(--safe-t) + 72px);
  --ss: 52px; --sx: 24px; --sy: calc(var(--safe-t) + 64px); --ly-top: calc(var(--sy) + 76px); --ctl-h: 184px;
}
.kp[data-immersive] .stage {
  inset: 0; width: auto; height: auto; border-radius: 0;
  --cs: min(440px, 50vh); --cx: max(64px, calc(50vw - 560px)); --cy: calc(50vh - var(--cs) / 2 - 120px);
}
.amb { position: absolute; inset: 0; overflow: hidden; background: #2B3350; }
.amb img { position: absolute; left: -60%; top: -30%; width: 220%; height: 160%; filter: blur(64px) saturate(1.35) brightness(.72); transform: translateZ(0); transition: opacity .6s; }
.kp[data-immersive] .amb img { left: -20%; top: -40%; width: 140%; height: 180%; filter: blur(110px) saturate(1.35) brightness(.7); }
/* 白色、浅色封面会把底色洗白:整体压一层暗色,保证白字歌词的对比度。 */
.scrim { position: absolute; inset: 0; background: linear-gradient(180deg, rgba(14,14,26,.34) 0%, rgba(14,14,26,.22) 42%, rgba(14,14,26,.52) 100%); }
.top { position: absolute; z-index: 3; left: 10px; right: 10px; top: calc(var(--safe-t) + 10px); height: 40px; display: flex; align-items: center; justify-content: space-between; }
.top .ib:hover, .ctl .ib:hover, .stage .ib:hover { background: rgba(255,255,255,.12); }
.dots { display: flex; gap: 6px; padding: 8px; }
.dots button { width: 5px; height: 5px; border-radius: 3px; background: rgba(255,255,255,.4); transition: width .35s var(--ease), background-color .35s; }
.kp[data-view="cover"] .dots button[data-go="cover"], .kp[data-view="lyrics"] .dots button[data-go="lyrics"] { width: 16px; background: rgba(255,255,255,.95); }
.kp[data-immersive] .dots { visibility: hidden; }
.kp[data-device="mobile"] .immersive { visibility: hidden; }
.cover {
  position: absolute; z-index: 2; left: var(--cx); top: var(--cy); width: var(--cs); height: var(--cs);
  border-radius: 18px; overflow: hidden; background: rgba(255,255,255,.08); box-shadow: 0 30px 60px -24px rgba(8,10,30,.6);
  transition: left .48s var(--ease), top .48s var(--ease), width .48s var(--ease), height .48s var(--ease), border-radius .48s var(--ease), box-shadow .48s var(--ease);
}
.kp[data-view="lyrics"]:not([data-immersive]) .cover { left: var(--sx); top: var(--sy); width: var(--ss); height: var(--ss); border-radius: 10px; box-shadow: 0 8px 18px -8px rgba(8,10,30,.55); }
.badge { position: absolute; left: 12px; bottom: 12px; display: flex; height: 22px; border-radius: 6px; overflow: hidden; font-size: 11px; line-height: 22px; font-weight: 600; transition: opacity .2s; }
.badge b { padding: 0 7px; background: #F4D59C; color: #4A320C; font-weight: 600; }
.badge span { padding: 0 7px; background: rgba(14,16,30,.48); color: #FFFFFF; }
.kp[data-view="lyrics"]:not([data-immersive]) .badge { opacity: 0; }

.pages { position: absolute; inset: 0; overflow: hidden; pointer-events: none; }
.page { position: absolute; inset: 0; transition: transform .35s var(--ease), opacity .35s var(--ease); }
.page > * { pointer-events: auto; }
.page-cover { transform: none; }
.page-lyrics { transform: translateX(100%); opacity: .4; }
.kp[data-view="lyrics"] .page-cover { transform: translateX(-100%); opacity: .4; }
.kp[data-view="lyrics"] .page-lyrics { transform: none; opacity: 1; }
.kp[data-immersive] .page-cover, .kp[data-immersive] .page-lyrics { transform: none; opacity: 1; }
.meta { position: absolute; left: var(--pad); right: var(--pad); top: calc(var(--cy) + var(--cs) + 24px); }
.meta-title { font-size: 22px; line-height: 30px; font-weight: 700; }
.meta-sub { display: flex; align-items: center; gap: 8px; margin-top: 4px; font-size: 14px; line-height: 20px; color: rgba(255,255,255,.72); }
.tag { flex: none; height: 16px; padding: 0 5px; border-radius: 4px; border: 1px solid rgba(255,255,255,.4); font-size: 10px; line-height: 14px; font-weight: 700; color: rgba(255,255,255,.85); }
.kp[data-immersive] .meta { left: var(--cx); right: auto; width: var(--cs); top: calc(var(--cy) + var(--cs) + 28px); }
.kp[data-immersive] .meta-title { font-size: 30px; line-height: 40px; }
.head { position: absolute; left: calc(var(--sx) + var(--ss) + 12px); right: 64px; top: var(--sy); height: var(--ss); display: flex; flex-direction: column; justify-content: center; text-align: left; }
.head-title { font-size: 16px; line-height: 22px; font-weight: 600; }
.head-sub { font-size: 13px; line-height: 18px; color: rgba(255,255,255,.72); }
.kp[data-immersive] .head { display: none; }

.lyrics {
  position: absolute; left: 0; right: 0; top: var(--ly-top); bottom: calc(var(--ctl-h) + var(--safe-b) + 24px);
  overflow: hidden; touch-action: pan-x;
  -webkit-mask-image: linear-gradient(180deg, transparent 0, #000 11%, #000 80%, transparent 100%); mask-image: linear-gradient(180deg, transparent 0, #000 11%, #000 80%, transparent 100%);
  transition: bottom .4s var(--ease);
  --lsize: 22px; --tsize: 15px;
}
.kp[data-idle] .lyrics { bottom: calc(var(--safe-b) + 24px); }
.kp[data-immersive] .lyrics { left: calc(var(--cx) + var(--cs) + 96px); right: 48px; top: 10vh; bottom: 10vh; --lsize: 34px; --tsize: 20px; }
.line { position: absolute; left: 18px; right: 40px; top: 0; padding: 6px 10px; border-radius: 12px; text-align: left; transition: transform .6s var(--lyric), opacity .4s, filter .4s, background-color .16s; will-change: transform; }
.line:hover { background: rgba(255,255,255,.08); }
.lt { display: block; font-size: var(--lsize); line-height: 1.36; font-weight: 700; color: rgba(255,255,255,.42); transform-origin: 0 50%; transform: scale(.82); transition: transform .36s var(--ease), color .36s; }
.tr { display: block; margin-top: 3px; font-size: var(--tsize); line-height: 1.45; font-weight: 600; color: rgba(255,255,255,.42); transform-origin: 0 50%; transform: scale(.87); transition: transform .36s var(--ease), color .36s; }
.line.cur .lt { transform: none; color: rgba(255,255,255,.5); }
.line.cur .tr { transform: none; color: rgba(255,255,255,.9); }
.line.cur .u { color: transparent; background-image: linear-gradient(90deg, #FFFFFF 50%, rgba(255,255,255,.5) 50%); background-size: 200% 100%; background-position: calc((1 - var(--p, 0)) * 100%) 0; -webkit-background-clip: text; background-clip: text; }
.line.far { opacity: .5; filter: blur(1.2px); }
.kp:not([data-trans]) .tr { display: none; }
.lyr-empty { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 16px; font-size: 15px; color: rgba(255,255,255,.72); }
.lyr-empty .eq { height: 36px; gap: 4px; }
.lyr-empty .eq i { width: 6px; border-radius: 3px; background: rgba(255,255,255,.8); }
.seek { position: absolute; right: 14px; display: flex; align-items: center; gap: 4px; height: 22px; padding: 0 8px; border-radius: 11px; background: rgba(255,255,255,.18); font-size: 11px; font-weight: 600; font-variant-numeric: tabular-nums; opacity: 0; transition: opacity .2s; }
.seek svg { width: 9px; height: 9px; }
.lyrics.manual .seek { opacity: 1; }

.ctl { position: absolute; z-index: 3; left: var(--pad); right: var(--pad); bottom: calc(var(--safe-b) + 24px); transition: opacity .3s var(--ease), transform .4s var(--ease); }
.kp[data-idle] .ctl { opacity: 0; transform: translateY(24px); pointer-events: none; }
.kp[data-immersive] .ctl { left: var(--cx); right: auto; width: var(--cs); top: calc(var(--cy) + var(--cs) + 108px); bottom: auto; }
.progress { position: relative; height: 24px; }
.ptrack, .pchorus, .pfill { position: absolute; top: 10px; height: 3px; border-radius: 2px; transition: height .16s, top .16s; }
.ptrack { left: 0; right: 0; background: rgba(255,255,255,.22); }
.pchorus { background: rgba(255,255,255,.38); }
.pfill { left: 0; background: #FFFFFF; width: calc(var(--pct, 0) * 100%); }
.pthumb { position: absolute; top: 5px; left: calc(var(--pct, 0) * 100%); width: 13px; height: 13px; margin-left: -6.5px; border-radius: 50%; background: #FFFFFF; box-shadow: 0 2px 6px rgba(0,0,0,.25); transform: scale(0); transition: transform .18s var(--ease); }
.progress:hover .pthumb, .progress:focus-within .pthumb { transform: scale(1); }
.progress:hover .ptrack, .progress:hover .pfill, .progress:hover .pchorus { height: 5px; top: 9px; }
.progress input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
.times { display: flex; justify-content: space-between; font-size: 11px; line-height: 14px; color: rgba(255,255,255,.72); font-variant-numeric: tabular-nums; }
.buttons { display: flex; align-items: center; justify-content: space-between; margin-top: 16px; }
.buttons .big svg { width: 30px; height: 30px; }
.buttons .blob { width: 68px; height: 68px; }
.buttons .blob svg { filter: drop-shadow(0 10px 18px rgba(0,0,0,.18)); }
.stage .blob .blob-shape { fill: #FFFFFF; }
.stage .blob .i-play, .stage .blob .i-pause { fill: #1C1B22; }
.kp[data-device="mobile"] .buttons .blob { width: 76px; height: 76px; }
.extra { display: flex; align-items: center; justify-content: space-between; margin-top: 12px; color: rgba(255,255,255,.72); }
.extra .ib svg { width: 20px; height: 20px; }
.extra .ib[disabled] { opacity: .3; cursor: default; }
.kp[data-trans] .trans-btn { color: #FFFFFF; }
.tip { position: absolute; left: 20px; bottom: 112px; padding: 6px 10px; border-radius: 8px; background: rgba(255,255,255,.95); color: #15171C; font-size: 12px; white-space: nowrap; pointer-events: none; opacity: 0; transform: translateX(-50%) translateY(6px) scale(.9); transition: opacity .16s, transform .32s var(--spring); }
.tip.on { opacity: 1; transform: translateX(-50%); }
.mode .m { display: none; }
.kp[data-pm="sequence"] .mode .m-sequence, .kp[data-pm="loop"] .mode .m-loop, .kp[data-pm="one"] .mode .m-one, .kp[data-pm="shuffle"] .mode .m-shuffle { display: grid; animation: pop .46s var(--spring); }
.kp[data-pm="sequence"] .mode { color: rgba(255,255,255,.6); }
@keyframes pop { from { transform: scale(.5) rotate(-90deg); opacity: 0; } to { transform: none; opacity: 1; } }

/* ---------- 播放列表抽屉 ---------- */
.backdrop { position: fixed; z-index: 2147482800; inset: 0; background: rgba(10,12,18,.42); opacity: 0; pointer-events: none; transition: opacity .3s; }
.kp[data-sheet] .backdrop { opacity: 1; pointer-events: auto; }
.sheet {
  position: fixed; z-index: 2147482900; right: var(--kite-offset); bottom: calc(var(--kite-offset) + 84px);
  width: 400px; height: min(520px, calc(100vh - 140px)); border-radius: 24px; overflow: hidden;
  background: var(--kite-sheet); color: var(--kite-fg); box-shadow: 0 -12px 40px -12px rgba(10,12,18,.3);
  display: flex; flex-direction: column; transform: translateY(calc(100% + 120px)); visibility: hidden;
  transition: transform .4s cubic-bezier(.2,1.04,.3,1), visibility 0s .4s;
}
.kp[data-mode="open"] .sheet { bottom: var(--kite-offset); border-radius: 24px; }
.kp[data-sheet] .sheet { transform: none; visibility: visible; transition: transform .4s cubic-bezier(.2,1.04,.3,1), visibility 0s; }
.kp[data-device="mobile"] .sheet { left: 0; right: 0; bottom: 0; width: auto; height: min(72vh, 620px); border-radius: 24px 24px 0 0; padding-bottom: env(safe-area-inset-bottom, 0px); }
.sheet-head { flex: none; position: relative; padding: 22px 20px 0; }
.handle { position: absolute; left: 50%; top: 8px; width: 36px; height: 5px; margin-left: -18px; border-radius: 3px; background: var(--kite-chip-strong); }
.sheet-title { display: flex; align-items: center; justify-content: space-between; font-size: 18px; line-height: 26px; font-weight: 700; }
.sheet-title .ib { width: 32px; height: 32px; border-radius: 16px; background: var(--kite-chip); }
.sheet-title .ib svg { width: 16px; height: 16px; }
.tabs { position: relative; display: grid; grid-auto-flow: column; grid-auto-columns: minmax(0, 1fr); margin-top: 14px; padding: 4px; height: 38px; border-radius: 12px; background: var(--kite-chip); }
.tabs .ind { position: absolute; left: 4px; top: 4px; bottom: 4px; border-radius: 9px; background: var(--kite-sheet); box-shadow: 0 1px 3px rgba(10,12,18,.14); transition: transform .46s var(--spring), width .46s var(--spring); }
.tabs button { position: relative; min-width: 0; padding: 0 6px; font-size: 13px; color: var(--kite-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.tabs button[aria-selected="true"] { color: var(--kite-fg); font-weight: 600; }
.tabs em { font-style: normal; margin-left: 3px; font-size: 11px; opacity: .7; }
.moderow { display: flex; align-items: center; gap: 8px; height: 40px; margin-top: 8px; font-size: 14px; font-weight: 600; }
.moderow svg { width: 18px; height: 18px; }
.moderow .src { margin-left: auto; font-size: 12px; font-weight: 400; color: var(--kite-muted); }
.list { flex: 1; overflow-y: auto; padding: 0 8px 16px; overscroll-behavior: contain; }
.row { display: flex; align-items: center; gap: 12px; width: 100%; min-height: 58px; padding: 8px 12px; border-radius: 12px; text-align: left; transition: background-color .16s; animation: rowin .46s var(--ease) both; animation-delay: calc(var(--i, 0) * 30ms); }
.row:hover { background: var(--kite-chip); }
@keyframes rowin { from { opacity: 0; transform: translateY(10px); } to { opacity: 1; transform: none; } }
.row-main { flex: 1; min-width: 0; }
.row-title { display: flex; align-items: center; gap: 6px; font-size: 15px; line-height: 22px; font-weight: 600; }
.row-sub { display: flex; align-items: center; gap: 6px; margin-top: 1px; font-size: 12px; line-height: 17px; color: var(--kite-muted); white-space: nowrap; overflow: hidden; }
.row-sub .ell { min-width: 0; }
.row-dur { flex: none; font-size: 12px; color: var(--kite-muted); font-variant-numeric: tabular-nums; }
.row.cur .row-title, .row.cur .row-sub { color: var(--kite-accent); }
.row.locked { cursor: not-allowed; }
.row.locked .row-title { opacity: .45; }
.row .lock svg { width: 17px; height: 17px; color: var(--kite-muted); opacity: .7; }
.rtag { flex: none; height: 15px; padding: 0 4px; border-radius: 4px; font-size: 10px; line-height: 14px; font-weight: 700; }
.rtag.vip { background: #F6E7C8; color: #7A5310; }
.rtag.paid { border: 1px solid currentColor; }
.empty { padding: 40px 20px; text-align: center; font-size: 13px; color: var(--kite-muted); }

/* ---------- 提示、飞入 ---------- */
.toast { position: fixed; z-index: 2147483000; left: 50%; top: 16px; display: flex; align-items: center; gap: 8px; max-width: calc(100vw - 32px); padding: 10px 16px; border-radius: 999px; background: rgba(21,23,28,.92); color: #FFFFFF; font-size: 13px; box-shadow: 0 18px 50px -18px rgba(0,0,0,.5); transform: translate(-50%, -14px); opacity: 0; pointer-events: none; transition: transform .38s var(--spring), opacity .2s; }
.toast.on { transform: translate(-50%, 0); opacity: 1; }
.ghost { position: fixed; z-index: 2147483100; left: 0; top: 0; pointer-events: none; }
.ghost-in { overflow: hidden; box-shadow: 0 12px 30px -10px rgba(0,0,0,.45); }

@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { animation-duration: 1ms !important; animation-iteration-count: 1 !important; transition-duration: 160ms !important; transition-delay: 0ms !important; }
  .disc-img img { animation: none !important; }
  .line.far { filter: none; }
}
`;
