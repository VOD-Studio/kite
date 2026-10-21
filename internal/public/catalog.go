package public

import (
	"context"
	"strconv"
	"time"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/compose"
	playlistendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/playlist"
	songendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/song"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// catalog 是公开模式读数据的来源。生产实现直连 engine + endpoint,测试替换为假实现。
type catalog interface {
	Playlist(ctx context.Context, id int64) (*mmpb.Playlist, error)
	Song(ctx context.Context, id int64) (*mmpb.Song, error)
	Lyric(ctx context.Context, id int64) (*mmpb.Lyric, error)
	WordLyric(ctx context.Context, id int64) (*mmpb.WordLyric, error)
	Chorus(ctx context.Context, id int64) ([]*mmpb.ChorusSegment, error)
	// StreamURL 取上游播放直链信息(含试听标记 trial/trialStartMs/trialEndMs);
	// cookie 为空时按匿名取(VIP / 版权受限的歌返回空串)。
	StreamURL(ctx context.Context, id int64, level mmpb.SongLevel, cookie string) (*mmpb.SongURL, error)
}

// engineCatalog 用 engine 执行 endpoint 声明。执行语义只在 engine 定义,这里只选 endpoint。
type engineCatalog struct {
	eng *engine.Engine
}

// Playlist 取歌单信息与歌曲。详情内嵌的歌曲会被上游截断(匿名时约 10 首),
// 歌曲改由 compose.PlaylistTracks 按完整 trackIds 取,一个歌单最多 compose.MaxPageSize 首。
func (c engineCatalog) Playlist(ctx context.Context, id int64) (*mmpb.Playlist, error) {
	resp, err := engine.Execute(c.eng, ctx, playlistendpoint.GetPlaylist, &mmpb.GetPlaylistRequest{PlaylistId: id})
	if err != nil {
		return nil, err
	}
	tracks, err := compose.PlaylistTracks(ctx, c.eng, &mmpb.AllTracksRequest{PlaylistId: id, Limit: compose.MaxPageSize})
	if err != nil {
		return nil, err
	}
	pl := resp.GetPlaylist()
	pl.Songs = tracks.GetSongs()
	return pl, nil
}

func (c engineCatalog) Song(ctx context.Context, id int64) (*mmpb.Song, error) {
	resp, err := engine.Execute(c.eng, ctx, songendpoint.Detail, &mmpb.GetSongDetailRequest{SongId: id})
	if err != nil {
		return nil, err
	}
	return resp.GetSong(), nil
}

func (c engineCatalog) Lyric(ctx context.Context, id int64) (*mmpb.Lyric, error) {
	resp, err := engine.Execute(c.eng, ctx, songendpoint.Lyric, &mmpb.GetLyricRequest{SongId: id})
	if err != nil {
		return nil, err
	}
	return resp.GetLyric(), nil
}

func (c engineCatalog) WordLyric(ctx context.Context, id int64) (*mmpb.WordLyric, error) {
	resp, err := engine.Execute(c.eng, ctx, songendpoint.WordLyricEP, &mmpb.GetWordLyricRequest{SongId: id})
	if err != nil {
		return nil, err
	}
	return resp.GetWordLyric(), nil
}

func (c engineCatalog) Chorus(ctx context.Context, id int64) ([]*mmpb.ChorusSegment, error) {
	resp, err := engine.Execute(c.eng, ctx, songendpoint.ChorusTime, &mmpb.ChorusTimeRequest{SongId: id})
	if err != nil {
		return nil, err
	}
	return resp.GetSegments(), nil
}

func (c engineCatalog) StreamURL(ctx context.Context, id int64, level mmpb.SongLevel, cookie string) (*mmpb.SongURL, error) {
	// ExecuteWithCookie:凭据只来自这里注入的 cookie(空即匿名),不经会话池,也不读写 engine 缓存
	// (直链几十分钟就过期,缓存由 cachedCatalog 用更短的 TTL 控制)。
	resp, err := engine.ExecuteWithCookie(c.eng, engine.WithCookie(ctx, cookie), songendpoint.URL,
		&mmpb.GetSongURLRequest{SongId: id, Level: level})
	if err != nil {
		return nil, err
	}
	return resp.GetUrl(), nil
}

// 各类数据在公开模式里的缓存时间。歌单短一些,歌单改了能较快生效;直链远短于上游的过期时间。
const (
	playlistTTL = 10 * time.Minute
	songTTL     = 6 * time.Hour
	lyricTTL    = 24 * time.Hour
	streamTTL   = 4 * time.Minute
)

// cachedCatalog 给 catalog 加进程内缓存。只缓存成功结果,上游出错时下次请求会重试。
type cachedCatalog struct {
	next     catalog
	playlist *ttlCache[*mmpb.Playlist]
	song     *ttlCache[*mmpb.Song]
	lyric    *ttlCache[*mmpb.Lyric]
	word     *ttlCache[*mmpb.WordLyric]
	chorus   *ttlCache[[]*mmpb.ChorusSegment]
	stream   *ttlCache[*mmpb.SongURL]
}

func newCachedCatalog(next catalog, now func() time.Time) *cachedCatalog {
	return &cachedCatalog{
		next:     next,
		playlist: newTTLCache[*mmpb.Playlist](64, now),
		song:     newTTLCache[*mmpb.Song](4096, now),
		lyric:    newTTLCache[*mmpb.Lyric](2048, now),
		word:     newTTLCache[*mmpb.WordLyric](2048, now),
		chorus:   newTTLCache[[]*mmpb.ChorusSegment](2048, now),
		stream:   newTTLCache[*mmpb.SongURL](2048, now),
	}
}

func cached[V any](c *ttlCache[V], key string, ttl time.Duration, load func() (V, error)) (V, error) {
	if v, ok := c.get(key); ok {
		return v, nil
	}
	v, err := load()
	if err != nil {
		return v, err
	}
	c.set(key, v, ttl)
	return v, nil
}

func idKey(id int64) string { return strconv.FormatInt(id, 10) }

func (c *cachedCatalog) Playlist(ctx context.Context, id int64) (*mmpb.Playlist, error) {
	return cached(c.playlist, idKey(id), playlistTTL, func() (*mmpb.Playlist, error) { return c.next.Playlist(ctx, id) })
}

func (c *cachedCatalog) Song(ctx context.Context, id int64) (*mmpb.Song, error) {
	return cached(c.song, idKey(id), songTTL, func() (*mmpb.Song, error) { return c.next.Song(ctx, id) })
}

func (c *cachedCatalog) Lyric(ctx context.Context, id int64) (*mmpb.Lyric, error) {
	return cached(c.lyric, idKey(id), lyricTTL, func() (*mmpb.Lyric, error) { return c.next.Lyric(ctx, id) })
}

func (c *cachedCatalog) WordLyric(ctx context.Context, id int64) (*mmpb.WordLyric, error) {
	return cached(c.word, idKey(id), lyricTTL, func() (*mmpb.WordLyric, error) { return c.next.WordLyric(ctx, id) })
}

func (c *cachedCatalog) Chorus(ctx context.Context, id int64) ([]*mmpb.ChorusSegment, error) {
	return cached(c.chorus, idKey(id), lyricTTL, func() ([]*mmpb.ChorusSegment, error) { return c.next.Chorus(ctx, id) })
}

func (c *cachedCatalog) StreamURL(ctx context.Context, id int64, level mmpb.SongLevel, cookie string) (*mmpb.SongURL, error) {
	// 有无登录态拿到的链接不同(VIP 歌匿名时为空),按凭据分开缓存;不把 cookie 本身放进 key。
	scope := "anon"
	if cookie != "" {
		scope = "owner"
	}
	key := idKey(id) + ":" + strconv.Itoa(int(level)) + ":" + scope
	return cached(c.stream, key, streamTTL, func() (*mmpb.SongURL, error) { return c.next.StreamURL(ctx, id, level, cookie) })
}
