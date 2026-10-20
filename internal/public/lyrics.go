package public

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// lrcLine 是 LRC 里的一行:时间戳(毫秒)+ 文本。
type lrcLine struct {
	ms   int64
	text string
}

// lrcTime 匹配 [mm:ss] / [mm:ss.xx] / [mm:ss.xxx] / [mm:ss:xx]。
var lrcTime = regexp.MustCompile(`\[(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?\]`)

// parseLRC 解析 LRC 文本。一行可带多个时间戳;[ar:]、[ti:] 等标签和网易云的 JSON 署名行会被忽略。
func parseLRC(raw string) []lrcLine {
	var out []lrcLine
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "{") {
			continue
		}
		stamps := lrcTime.FindAllStringSubmatchIndex(line, -1)
		if len(stamps) == 0 || stamps[0][0] != 0 {
			continue
		}
		// 时间戳都在行首连续出现,文本从最后一个时间戳之后开始。
		textStart := 0
		var times []int64
		for _, m := range stamps {
			if m[0] != textStart {
				break
			}
			times = append(times, stampMS(line, m))
			textStart = m[1]
		}
		text := strings.TrimSpace(line[textStart:])
		for _, t := range times {
			out = append(out, lrcLine{ms: t, text: text})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ms < out[j].ms })
	return out
}

func stampMS(line string, m []int) int64 {
	minutes, _ := strconv.ParseInt(line[m[2]:m[3]], 10, 64)
	seconds, _ := strconv.ParseInt(line[m[4]:m[5]], 10, 64)
	var frac int64
	if m[6] >= 0 {
		f := line[m[6]:m[7]]
		frac, _ = strconv.ParseInt(f, 10, 64)
		switch len(f) {
		case 1:
			frac *= 100
		case 2:
			frac *= 10
		}
	}
	return minutes*60_000 + seconds*1000 + frac
}

// matchTolerance 是原文与翻译时间戳对齐的容差:网易云的翻译行偶尔差几十毫秒。
const matchTolerance = 600

// nearest 在按时间排序的行里找与 ms 最接近、且在容差内的一行文本。
func nearest(lines []lrcLine, ms int64) string {
	i := sort.Search(len(lines), func(i int) bool { return lines[i].ms >= ms })
	best, bestDiff := "", int64(matchTolerance+1)
	for _, j := range []int{i - 1, i} {
		if j < 0 || j >= len(lines) {
			continue
		}
		d := lines[j].ms - ms
		if d < 0 {
			d = -d
		}
		if d < bestDiff {
			best, bestDiff = lines[j].text, d
		}
	}
	if bestDiff > matchTolerance {
		return ""
	}
	return best
}

// buildLyrics 把逐字歌词、LRC 原文、翻译、音译合成播放器直接可用的行。
// 有逐字歌词时以它为准(带每个字的时间),否则用 LRC;翻译与音译按时间对齐挂到对应行。
func buildLyrics(lyric *mmpb.Lyric, word *mmpb.WordLyric, chorus []*mmpb.ChorusSegment) lyricsDTO {
	trans := parseLRC(lyric.GetTranslated())
	roma := parseLRC(lyric.GetRomanized())
	out := lyricsDTO{Lines: []lineDTO{}, Chorus: []segmentDTO{}}

	if lines := word.GetLines(); len(lines) > 0 {
		for _, l := range lines {
			if strings.TrimSpace(l.GetContent()) == "" {
				continue
			}
			d := lineDTO{Time: l.GetStartMs(), Duration: l.GetDurationMs(), Text: l.GetContent()}
			for _, w := range l.GetWords() {
				d.Words = append(d.Words, wordDTO{Time: w.GetStartMs(), Duration: w.GetDurationMs(), Text: w.GetContent()})
			}
			d.Translation = nearest(trans, d.Time)
			d.Romanized = nearest(roma, d.Time)
			out.Lines = append(out.Lines, d)
		}
	} else {
		src := parseLRC(lyric.GetLrc())
		for i, l := range src {
			if l.text == "" {
				continue
			}
			d := lineDTO{Time: l.ms, Text: l.text}
			if i+1 < len(src) {
				d.Duration = src[i+1].ms - l.ms
			}
			d.Translation = nearest(trans, l.ms)
			d.Romanized = nearest(roma, l.ms)
			out.Lines = append(out.Lines, d)
		}
	}

	out.Instrumental = isInstrumental(out.Lines)
	if out.Instrumental {
		out.Lines = []lineDTO{}
	}
	for _, s := range chorus {
		out.Chorus = append(out.Chorus, segmentDTO{Start: s.GetStartMs(), End: s.GetEndMs()})
	}
	return out
}

// isInstrumental 判断是否是纯音乐:没有歌词,或者只有「纯音乐,请欣赏」这类提示行。
func isInstrumental(lines []lineDTO) bool {
	if len(lines) == 0 {
		return true
	}
	if len(lines) > 2 {
		return false
	}
	for _, l := range lines {
		if strings.Contains(l.Text, "纯音乐") {
			return true
		}
	}
	return false
}
