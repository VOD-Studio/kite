// <kite-track> / <kite-playlist> 的样式:小卡片(s)、标准卡片(m)、大卡片(l)。
// 卡片按自身宽度切换紧凑布局(container query),手机与桌面共用一套。

export const cardStyles = /* css */ `
:host {
  --kite-accent: #D33A40;
  --kite-font: -apple-system, BlinkMacSystemFont, "PingFang SC", "Hiragino Sans GB", "Noto Sans SC", "Microsoft YaHei", system-ui, sans-serif;
  --kite-card: #FFFFFF;
  --kite-fg: #15171C;
  --kite-muted: #5D6370;
  --kite-line: rgba(21,23,28,.07);
  --kite-chip: #F1F2F4;
  --kite-track: rgba(21,23,28,.08);
  --kite-blob: #15171C;
  --kite-blob-icon: #FFFFFF;
  --ease: cubic-bezier(.2,.85,.25,1);
  --spring: cubic-bezier(.34,1.3,.5,1);
  display: block; margin: 24px 0; container: card / inline-size;
  font-family: var(--kite-font); color: var(--kite-fg); -webkit-font-smoothing: antialiased;
}
:host([theme="dark"]) { --kite-card: #17191E; --kite-fg: #ECEDEF; --kite-muted: #A9AEB9; --kite-line: rgba(255,255,255,.08); --kite-chip: rgba(255,255,255,.08); --kite-track: rgba(255,255,255,.12); --kite-blob: #F2F3F5; --kite-blob-icon: #15171C; }
@media (prefers-color-scheme: dark) {
  :host(:not([theme="light"])) { --kite-card: #17191E; --kite-fg: #ECEDEF; --kite-muted: #A9AEB9; --kite-line: rgba(255,255,255,.08); --kite-chip: rgba(255,255,255,.08); --kite-track: rgba(255,255,255,.12); --kite-blob: #F2F3F5; --kite-blob-icon: #15171C; }
}
* { box-sizing: border-box; }
button { font: inherit; color: inherit; background: none; border: 0; padding: 0; margin: 0; cursor: pointer; -webkit-tap-highlight-color: transparent; }
button:focus-visible, input:focus-visible { outline: 2px solid var(--kite-accent); outline-offset: 2px; }
img { display: block; width: 100%; height: 100%; object-fit: cover; }
[hidden] { display: none !important; }
.ell { display: block; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
.mut { color: var(--kite-muted); }
.num { font-variant-numeric: tabular-nums; }

.card { position: relative; overflow: hidden; border-radius: 20px; background: var(--kite-card); box-shadow: 0 0 0 1px var(--kite-line), 0 10px 30px -22px rgba(21,23,28,.3); transition: box-shadow .3s var(--ease), background-color .3s; }
.card.cur { background: linear-gradient(90deg, color-mix(in srgb, var(--kite-accent) 7%, transparent), transparent 60%), var(--kite-card); box-shadow: 0 0 0 1px color-mix(in srgb, var(--kite-accent) 20%, transparent), 0 14px 34px -22px color-mix(in srgb, var(--kite-accent) 45%, transparent); }
.cover { position: relative; flex: none; overflow: hidden; border-radius: 14px; background: #D9DCE2; }
.locked .cover { filter: grayscale(.75); opacity: .72; }
.ib { display: inline-grid; place-items: center; width: 36px; height: 36px; border-radius: 18px; flex: none; transition: background-color .16s, transform .16s var(--ease); }
.ib svg { width: 22px; height: 22px; }
.ib:hover { background: var(--kite-chip); }
.ib:active { transform: scale(.9); }
.chip { display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 12px; border-radius: 15px; background: var(--kite-chip); font-size: 12.5px; color: var(--kite-muted); white-space: nowrap; flex: none; transition: background-color .16s, color .16s; }
.chip:hover { color: var(--kite-fg); }
.chip svg { width: 14px; height: 14px; }
.blob { position: relative; display: grid; place-items: center; flex: none; width: 46px; height: 46px; transition: transform .16s var(--ease); }
.blob:active { transform: scale(.92); }
.blob svg { position: absolute; inset: 0; width: 100%; height: 100%; }
.blob .blob-shape { fill: var(--kite-blob); }
.blob .i-play, .blob .i-pause { fill: var(--kite-blob-icon); transform-origin: 32px 30px; transition: opacity .2s, transform .42s var(--spring); }
.blob .i-pause { opacity: 0; transform: scale(.4) rotate(-90deg); }
.playing .blob .i-play { opacity: 0; transform: scale(.4) rotate(90deg); }
.playing .blob .i-pause { opacity: 1; transform: none; }
.locked .blob .blob-shape { fill: var(--kite-chip); }
.locked .blob .i-play { fill: var(--kite-muted); }
.eq { display: inline-flex; align-items: flex-end; gap: 2px; height: 13px; margin-right: 8px; flex: none; }
.eq i { display: block; width: 2.5px; height: 100%; border-radius: 2px; background: var(--kite-accent); transform-origin: 50% 100%; transform: scaleY(.35); animation: eq 900ms ease-in-out infinite alternate; animation-play-state: paused; }
.eq i:nth-child(2) { animation-duration: 700ms; animation-delay: -300ms; }
.eq i:nth-child(3) { animation-duration: 1000ms; animation-delay: -600ms; }
.playing .eq i { animation-play-state: running; }
.card:not(.cur) .eq { display: none; }
@keyframes eq { from { transform: scaleY(.25); } to { transform: scaleY(1); } }
.tg { display: inline-block; flex: none; height: 16px; padding: 0 5px; border-radius: 4px; font-size: 10px; line-height: 15px; font-weight: 700; }
.tg.vip { background: #F6E7C8; color: #7A5310; }
.tg.out { border: 1px solid color-mix(in srgb, var(--kite-accent) 55%, transparent); color: var(--kite-accent); }
.progress { position: relative; display: flex; align-items: center; gap: 10px; font-size: 12px; line-height: 16px; color: var(--kite-muted); }
.bar { position: relative; flex: 1; height: 4px; border-radius: 2px; background: var(--kite-track); overflow: hidden; }
.bar b { position: absolute; left: 0; top: 0; bottom: 0; width: calc(var(--pct, 0) * 100%); border-radius: 2px; background: var(--kite-accent); }
.progress input { position: absolute; inset: -8px 0; width: 100%; height: calc(100% + 16px); margin: 0; opacity: 0; cursor: pointer; }
.k1 { color: var(--kite-fg); }
.lyric-line { font-weight: 600; }
.lyric-line .u { color: transparent; background-image: linear-gradient(90deg, var(--kite-fg) 50%, color-mix(in srgb, var(--kite-fg) 32%, transparent) 50%); background-size: 200% 100%; background-position: calc((1 - var(--p, 0)) * 100%) 0; -webkit-background-clip: text; background-clip: text; }
.error { margin-top: 4px; font-size: 12.5px; color: var(--kite-muted); }

/* 小卡片 */
.s { display: flex; align-items: center; gap: 18px; height: 96px; padding: 12px 18px 12px 12px; }
.s .cover { width: 72px; height: 72px; }
.s .main { flex: 1; min-width: 0; }
.s .title { display: flex; align-items: center; font-size: 17px; line-height: 24px; font-weight: 650; }
.s .title .who { margin-left: 10px; font-size: 13.5px; font-weight: 400; color: var(--kite-muted); }
.s .sub { display: flex; align-items: center; gap: 6px; margin-top: 4px; font-size: 13.5px; line-height: 20px; color: var(--kite-muted); }
.s .sub svg { width: 15px; height: 15px; flex: none; }
.s .progress { margin-top: 10px; }
.s .actions { display: flex; align-items: center; gap: 10px; }
@container card (max-width: 480px) {
  .s { height: 80px; gap: 14px; padding: 12px 14px 12px 12px; }
  .s .cover { width: 56px; height: 56px; border-radius: 12px; }
  .s .title { font-size: 16px; }
  .s .title .who { font-size: 12.5px; margin-left: 6px; }
  .s .sub { font-size: 12.5px; }
  .s .chip { display: none; }
  .s .blob { width: 42px; height: 42px; }
}

/* 标准卡片 */
.m { display: grid; grid-template-columns: 176px minmax(0, 1fr); gap: 24px; padding: 20px; }
.m .cover { width: 176px; height: 176px; border-radius: 16px; box-shadow: 0 14px 30px -16px rgba(20,30,70,.55); }
.m .main { display: flex; flex-direction: column; min-width: 0; }
.m .head { display: flex; align-items: flex-start; gap: 12px; }
.m .title { font-size: 22px; line-height: 30px; font-weight: 700; }
.m .who { display: flex; align-items: center; gap: 8px; margin-top: 2px; font-size: 14px; line-height: 20px; color: var(--kite-muted); }
.m .lyric { margin-top: 14px; min-height: 45px; }
.m .lyric-line { font-size: 16px; line-height: 24px; }
.m .lyric-tr { font-size: 13px; line-height: 19px; color: var(--kite-muted); }
.m .controls { display: flex; align-items: center; gap: 6px; margin-top: auto; padding-top: 12px; }
.m .controls .progress { flex: 1; margin-left: 10px; }
@container card (max-width: 520px) {
  .m { grid-template-columns: 92px minmax(0, 1fr); gap: 14px; padding: 14px; }
  .m .cover { width: 92px; height: 92px; border-radius: 14px; }
  .m .title { font-size: 17px; line-height: 24px; }
  .m .who { font-size: 13px; }
  .m .chip { display: none; }
  .m .lyric { margin-top: 8px; min-height: 0; }
  .m .lyric-line { font-size: 13.5px; line-height: 20px; }
  .m .lyric-tr { display: none; }
  .m .controls .ib { display: none; }
  .m .controls .progress { margin-left: 4px; }
  .m .controls .blob { order: 2; width: 42px; height: 42px; }
}

/* 大卡片 */
.l { color: #FFFFFF; border-radius: 24px; padding: 28px; box-shadow: 0 24px 50px -28px rgba(20,30,70,.7); background: #33405F; }
.l .amb { position: absolute; inset: 0; overflow: hidden; }
.l .amb img { position: absolute; left: -40%; top: -60%; width: 180%; height: 220%; filter: blur(60px) saturate(1.35) brightness(.88); }
.l .scrim { position: absolute; inset: 0; background: linear-gradient(180deg, rgba(14,14,26,.12), rgba(14,14,26,.38)); }
.l > :not(.amb):not(.scrim) { position: relative; }
.l .top { display: flex; gap: 26px; align-items: flex-start; }
.l .cover { width: 168px; height: 168px; border-radius: 16px; box-shadow: 0 20px 40px -18px rgba(6,8,26,.65); }
.l .info { flex: 1; min-width: 0; }
.l .kicker { font-size: 12px; line-height: 18px; color: rgba(255,255,255,.72); letter-spacing: .04em; }
.l .title { margin-top: 4px; font-size: 28px; line-height: 36px; font-weight: 700; }
.l .who { margin-top: 4px; font-size: 15px; line-height: 22px; color: rgba(255,255,255,.75); }
.l .tags { display: flex; gap: 6px; margin-top: 10px; }
.l .tg.out { border-color: rgba(255,255,255,.45); color: rgba(255,255,255,.85); }
.l .chip { margin-top: 12px; background: rgba(255,255,255,.16); color: #FFFFFF; }
.l .lyric { margin-top: 26px; min-height: 94px; }
.l .lyric-prev, .l .lyric-next { font-size: 15px; line-height: 22px; font-weight: 600; color: rgba(255,255,255,.42); }
.l .lyric-line { margin-top: 4px; font-size: 21px; line-height: 30px; font-weight: 700; }
.l .lyric-line .u { background-image: linear-gradient(90deg, #FFFFFF 50%, rgba(255,255,255,.45) 50%); }
.l .lyric-line:not(.live) { color: rgba(255,255,255,.9); }
.l .lyric-tr { font-size: 14px; line-height: 20px; color: rgba(255,255,255,.85); }
.l .controls { display: flex; align-items: center; gap: 8px; margin-top: 18px; }
.l .controls .ib:hover { background: rgba(255,255,255,.14); }
.l .blob { width: 52px; height: 52px; }
.l .blob svg { filter: drop-shadow(0 8px 14px rgba(0,0,0,.18)); }
.l .blob .blob-shape { fill: #FFFFFF; }
.l .blob .i-play, .l .blob .i-pause { fill: #1C1B22; }
.l .progress { flex: 1; margin-left: 10px; color: rgba(255,255,255,.75); }
.l .progress .bar { height: 3px; background: rgba(255,255,255,.24); }
.l .progress .bar b { background: #FFFFFF; }
@container card (max-width: 520px) {
  .l { padding: 20px; border-radius: 22px; }
  .l .top { gap: 16px; }
  .l .cover { width: 112px; height: 112px; border-radius: 14px; }
  .l .title { font-size: 22px; line-height: 30px; }
  .l .who { font-size: 13.5px; }
  .l .chip { display: none; }
  .l .lyric-line { font-size: 19px; line-height: 27px; }
  .l .controls { flex-wrap: wrap; }
  .l .controls .progress { order: -1; flex-basis: 100%; margin: 0 0 12px; }
}

/* 歌单卡片 */
.pl { padding: 20px; }
.pl .head { display: flex; align-items: center; gap: 20px; }
.stack { position: relative; flex: none; width: 104px; height: 88px; }
.stack .cover { position: absolute; box-shadow: 0 0 0 2px var(--kite-card); }
.stack .c3 { left: 36px; top: 10px; width: 68px; height: 68px; border-radius: 13px; opacity: .5; }
.stack .c2 { left: 18px; top: 5px; width: 78px; height: 78px; border-radius: 14px; opacity: .8; }
.stack .c1 { left: 0; top: 0; width: 88px; height: 88px; border-radius: 16px; }
.pl .info { flex: 1; min-width: 0; }
.pl .kicker { font-size: 12px; line-height: 18px; color: var(--kite-muted); letter-spacing: .04em; }
.pl .name { margin-top: 2px; font-size: 21px; line-height: 28px; font-weight: 700; }
.pl .meta { margin-top: 4px; font-size: 13.5px; line-height: 20px; color: var(--kite-muted); }
.playall { display: inline-flex; align-items: center; gap: 6px; height: 38px; padding: 0 18px; border-radius: 19px; background: var(--kite-accent); color: #FFFFFF; font-size: 14px; font-weight: 600; flex: none; transition: filter .16s, transform .16s var(--ease); }
.playall:hover { filter: brightness(1.08); }
.playall:active { transform: scale(.95); }
.playall svg { width: 14px; height: 14px; }
.rows { margin-top: 14px; padding-top: 6px; border-top: 1px solid var(--kite-line); }
.row { display: grid; grid-template-columns: 28px minmax(0, 1fr) auto; align-items: center; gap: 12px; width: 100%; min-height: 48px; padding: 0 10px; border-radius: 12px; text-align: left; font-size: 14px; transition: background-color .16s; }
.row:hover { background: var(--kite-chip); }
.row .idx { color: var(--kite-muted); font-variant-numeric: tabular-nums; }
.row .t { display: flex; align-items: baseline; gap: 10px; min-width: 0; }
.row .t b { font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row .t span { font-size: 13px; color: var(--kite-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row .d { font-size: 12.5px; color: var(--kite-muted); font-variant-numeric: tabular-nums; }
.row.cur b, .row.cur .idx { color: var(--kite-accent); }
.row.locked b { opacity: .45; }
.more { display: inline-block; margin: 10px 10px 0; font-size: 13px; line-height: 20px; color: var(--kite-accent); font-weight: 600; }
.pl.big .rows { max-height: 432px; overflow-y: auto; }
@container card (max-width: 520px) {
  .pl { padding: 14px; }
  .pl .head { gap: 14px; }
  .stack { width: 84px; height: 72px; }
  .stack .c3 { left: 28px; top: 8px; width: 56px; height: 56px; }
  .stack .c2 { left: 14px; top: 4px; width: 64px; height: 64px; }
  .stack .c1 { width: 72px; height: 72px; border-radius: 13px; }
  .pl .name { font-size: 17px; line-height: 24px; }
  .pl .meta { font-size: 12.5px; }
  .playall { width: 44px; height: 44px; padding: 0; justify-content: center; border-radius: 22px; }
  .playall span { display: none; }
  .row .t { flex-direction: column; gap: 0; }
}

.skeleton { height: 96px; border-radius: 20px; background: linear-gradient(90deg, var(--kite-chip), var(--kite-card), var(--kite-chip)); background-size: 200% 100%; animation: shimmer 1.4s linear infinite; }
@keyframes shimmer { to { background-position: -200% 0; } }
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { animation-duration: 1ms !important; animation-iteration-count: 1 !important; transition-duration: 160ms !important; }
}
`;
