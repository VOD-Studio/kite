// Package public 是给网页播放器用的公开只读 HTTP 适配器(/player/v1)。
//
// 与 internal/server 平行:不要求访问令牌,所以只暴露白名单里的歌单与歌曲,
// 播放链接带 HMAC 签名并会过期,按来源 IP 限流,可选用站长本人的网易云登录态取 VIP 播放链接。
// 与 CLI 一样直连 engine + endpoint,不经 gRPC service。协议与部署见 docs/player.md。
package public

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/VOD-Studio/kite/config"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// Options 是公开模式的运行参数,由 ParseOptions 从 config.PublicConfig 解析。
type Options struct {
	// Addr 是监听地址。
	Addr string
	// Playlists 是允许公开的歌单 ID。
	Playlists []int64
	// Songs 是允许单独公开的歌曲 ID。
	Songs []int64
	// Origins 是允许跨域调用的站点;AnyOrigin 为 true 时不限。
	Origins []string
	// AnyOrigin 表示 Origins 里出现了 *。
	AnyOrigin bool
	// SessionFile 是站长登录态文件路径,空表示只按匿名播放。
	SessionFile string
	// SigningKey 是播放链接签名密钥。
	SigningKey []byte
	// RatePerMinute 是每个来源 IP 每分钟允许的请求数。
	RatePerMinute int
	// TrustProxy 为 true 时按 X-Forwarded-For 取来源 IP。
	TrustProxy bool
	// Level 是播放音质上限。
	Level mmpb.SongLevel
	// LinkTTL 是播放链接签名的有效期。
	LinkTTL time.Duration
}

// defaultLinkTTL 是签名播放链接的有效期:页面开着几个小时仍能播放,链接泄露后也会自然失效。
const defaultLinkTTL = 6 * time.Hour

// minSigningKeyLen 是签名密钥的最短字节数(HMAC-SHA256,太短的密钥可被穷举)。
const minSigningKeyLen = 16

var levels = map[string]mmpb.SongLevel{
	"standard": mmpb.SongLevel_SONG_LEVEL_STANDARD,
	"exhigh":   mmpb.SongLevel_SONG_LEVEL_EXHIGH,
	"lossless": mmpb.SongLevel_SONG_LEVEL_LOSSLESS,
	"hires":    mmpb.SongLevel_SONG_LEVEL_HRES,
}

// ParseOptions 校验并解析公开模式配置。任何一项不合法都返回错误,调用方应拒绝启动:
// 公开模式不鉴权,配置含糊时宁可起不来,也不暴露超出预期的内容。
func ParseOptions(c config.PublicConfig) (Options, error) {
	opts := Options{
		Addr:          strings.TrimSpace(c.Addr),
		SessionFile:   strings.TrimSpace(c.SessionFile),
		RatePerMinute: c.RatePerMinute,
		TrustProxy:    c.TrustProxy,
		LinkTTL:       defaultLinkTTL,
	}
	var err error
	if opts.Playlists, err = parseIDs("KITE_PUBLIC_PLAYLISTS", c.Playlists); err != nil {
		return Options{}, err
	}
	if opts.Songs, err = parseIDs("KITE_PUBLIC_SONGS", c.Songs); err != nil {
		return Options{}, err
	}
	if len(opts.Playlists) == 0 && len(opts.Songs) == 0 {
		return Options{}, errors.New("公开模式需要白名单:设置 KITE_PUBLIC_PLAYLISTS 或 KITE_PUBLIC_SONGS")
	}
	if opts.Origins, opts.AnyOrigin, err = parseOrigins(c.Origins); err != nil {
		return Options{}, err
	}
	if opts.RatePerMinute <= 0 {
		return Options{}, fmt.Errorf("KITE_PUBLIC_RATE_PER_MINUTE 必须大于 0,当前为 %d", opts.RatePerMinute)
	}
	level, ok := levels[strings.ToLower(strings.TrimSpace(c.Quality))]
	if !ok {
		return Options{}, fmt.Errorf("KITE_PUBLIC_QUALITY 只能是 standard / exhigh / lossless / hires,当前为 %q", c.Quality)
	}
	opts.Level = level
	if opts.SigningKey, err = loadSigningKey(c.SigningKeyFile); err != nil {
		return Options{}, err
	}
	return opts, nil
}

func parseIDs(name, raw string) ([]int64, error) {
	var ids []int64
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("%s 里的 %q 不是有效的 ID", name, part)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func parseOrigins(raw string) ([]string, bool, error) {
	var origins []string
	anyOrigin := false
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
			continue
		case part == "*":
			anyOrigin = true
		case strings.HasPrefix(part, "https://") || strings.HasPrefix(part, "http://"):
			origins = append(origins, strings.TrimSuffix(part, "/"))
		default:
			return nil, false, fmt.Errorf("KITE_PUBLIC_ORIGINS 里的 %q 应写成 https://域名 或 *", part)
		}
	}
	return origins, anyOrigin, nil
}

// loadSigningKey 读取签名密钥文件;未配置时随机生成(重启后旧的播放链接失效,播放器会重新取)。
func loadSigningKey(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("生成签名密钥: %w", err)
		}
		return key, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取签名密钥文件: %w", err)
	}
	key := []byte(strings.TrimSpace(string(b)))
	if len(key) < minSigningKeyLen {
		return nil, fmt.Errorf("签名密钥文件 %s 太短,至少 %d 字节(可用 openssl rand -hex 32 生成)", path, minSigningKeyLen)
	}
	return key, nil
}
