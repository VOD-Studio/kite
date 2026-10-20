import type { LyricLine } from "./types";

/** lineIndexAt 返回 timeMs 时正在唱的行(二分查找),还没到第一行时返回 -1。 */
export function lineIndexAt(lines: readonly LyricLine[], timeMs: number): number {
  let lo = 0;
  let hi = lines.length - 1;
  let ans = -1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if ((lines[mid]?.time ?? Infinity) <= timeMs) {
      ans = mid;
      lo = mid + 1;
    } else {
      hi = mid - 1;
    }
  }
  return ans;
}

/** 网易云歌词开头的署名行,如「作词 : 林间」「制作人 : 郑润泽」。 */
const creditLine = /^[^:：]{1,12}\s*[:：]\s*\S/;

/**
 * firstSungIndex 跳过开头连续的署名行,返回第一句歌词的下标,用于还没开始播放时的预览。
 * 只看开头:正文里带冒号的歌词(如「他说:等等」)不受影响。
 */
export function firstSungIndex(lines: readonly LyricLine[]): number {
  let i = 0;
  while (i < lines.length - 1 && creditLine.test(lines[i]?.text ?? "")) i++;
  return i;
}

/**
 * wordProgress 返回一行里每个字已唱的比例(0~1),用于逐字点亮。
 * 没有逐字时间时按整行时长均分。
 */
export function wordProgress(line: LyricLine, timeMs: number): number[] {
  if (line.words && line.words.length > 0) {
    return line.words.map((w) => clamp((timeMs - w.time) / Math.max(w.duration, 1)));
  }
  const chars = [...line.text];
  const duration = line.duration ?? chars.length * 300;
  const per = duration / Math.max(chars.length, 1);
  return chars.map((_, i) => clamp((timeMs - line.time - i * per) / per));
}

/** 一行要逐字点亮的单位:有逐字时间用字,否则按字符拆。 */
export function lineUnits(line: LyricLine): string[] {
  return line.words && line.words.length > 0 ? line.words.map((w) => w.text) : [...line.text];
}

function clamp(v: number): number {
  return v < 0 ? 0 : v > 1 ? 1 : v;
}

/** formatTime 把秒格式化成 m:ss。 */
export function formatTime(sec: number): string {
  if (!Number.isFinite(sec) || sec < 0) sec = 0;
  const s = Math.floor(sec);
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}
