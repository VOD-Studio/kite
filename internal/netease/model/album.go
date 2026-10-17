// Package model 的专辑实体映射与专辑详情解码。
package model

import (
	"encoding/json"
	"fmt"
	"time"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// rawAlbum 是网易云专辑的原始 JSON 结构。
//
// 在歌曲的 al 字段、专辑详情等处出现。
type rawAlbum struct {
	ID          int64     `json:"id"`          // 专辑ID
	Name        string    `json:"name"`        // 专辑名
	Pic         string    `json:"picUrl"`      // 封面URL
	PublishTime int64     `json:"publishTime"` // 发行时间(毫秒时间戳)
	Artist      rawArtist `json:"artist"`      // 歌手
}

// rawAlbumDetail 是网易云专辑详情接口的响应。
type rawAlbumDetail struct {
	Code  int `json:"code"` // 业务码
	Album struct {
		ID          int64       `json:"id"`          // 专辑ID
		Name        string      `json:"name"`        // 专辑名
		PicUrl      string      `json:"picUrl"`      // 封面URL
		PublishTime int64       `json:"publishTime"` // 发行时间(毫秒级 Unix 时间戳;曾误声明为 string,真实响应一解析就报错)
		Artists     []rawArtist `json:"artists"`     // 歌手数组
	} `json:"album"` // 专辑信息
	Songs []rawSong `json:"songs"` // 歌曲列表
}

// MapAlbum 把网易云原始专辑结构转成 proto Album。
func MapAlbum(a rawAlbum) *mmpb.Album {
	return &mmpb.Album{
		Id:          a.ID,
		Name:        a.Name,
		PicUrl:      a.Pic,
		PublishTime: formatPublishTime(a.PublishTime),
		Artist:      MapArtist(a.Artist),
	}
}

// neteaseZone 是网易云日期时间戳对齐的时区。发行日的零点是北京时间零点
// (如 747504000000 = UTC 1993-09-08 16:00 = 北京 1993-09-09 00:00)。
// 必须固定时区:用进程本地时区,UTC 以西的主机会把发行日显示成前一天,同一个 RPC 的返回值随服务器所在时区而变;
// 转 UTC 同样不对(同样差一天)。用 FixedZone 而不是 LoadLocation,避免依赖运行环境里有 tzdata(distroless 镜像等)。
var neteaseZone = time.FixedZone("CST", 8*60*60)

// formatPublishTime 把网易云毫秒级 Unix 时间戳转成可读日期；0 / 负数表示未知，返回空串。
func formatPublishTime(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).In(neteaseZone).Format("2006-01-02")
}

// MapAlbums 把网易云原始专辑数组转成 proto Album 列表。
// 新碟列表、收藏列表、最新专辑等列表接口复用。
func MapAlbums(in []rawAlbum) []*mmpb.Album {
	out := make([]*mmpb.Album, 0, len(in))
	for _, a := range in {
		out = append(out, MapAlbum(a))
	}
	return out
}

// DecodeAlbumDetail 解析专辑详情响应的原始 JSON。
func DecodeAlbumDetail(raw json.RawMessage) (*mmpb.Album, []*mmpb.Song, error) {
	var r rawAlbumDetail
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, nil, fmt.Errorf("解析专辑详情失败: %w", err)
	}
	a := r.Album
	var albumArtist *mmpb.Artist
	if len(a.Artists) > 0 {
		albumArtist = MapArtist(a.Artists[0])
	}
	album := &mmpb.Album{
		Id:          a.ID,
		Name:        a.Name,
		PicUrl:      a.PicUrl,
		PublishTime: formatPublishTime(a.PublishTime),
		Artist:      albumArtist,
	}
	return album, MapSongs(r.Songs), nil
}

// --- 专辑列表解码（新碟/最新/收藏列表等复用） ---

// rawAlbumList 是网易云专辑列表接口的通用响应（albums 字段名在不同接口一致）。
type rawAlbumList struct {
	Code   int        `json:"code"`   // 业务码
	Albums []rawAlbum `json:"albums"` // 专辑数组（newest/new/shelf/sublist 通用字段名）
	Total  int        `json:"total"`  // 总数（带分页的接口有）
	More   bool       `json:"more"`   // 是否还有更多（部分接口用此标记而非 total）
}

// DecodeAlbumList 解析专辑列表响应，只取专辑数组（最新专辑等不带分页的接口用）。
func DecodeAlbumList(raw json.RawMessage) ([]*mmpb.Album, error) {
	var r rawAlbumList
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("解析专辑列表失败: %w", err)
	}
	return MapAlbums(r.Albums), nil
}

// DecodeAlbumListWithTotal 解析带 total 的专辑列表响应（全部新碟/收藏列表用）。
func DecodeAlbumListWithTotal(raw json.RawMessage) ([]*mmpb.Album, int32, error) {
	var r rawAlbumList
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, 0, fmt.Errorf("解析专辑列表失败: %w", err)
	}
	return MapAlbums(r.Albums), int32(r.Total), nil
}

// DecodeAlbumListWithMore 解析带 more 标记的专辑列表响应（新碟上架用）。
func DecodeAlbumListWithMore(raw json.RawMessage) ([]*mmpb.Album, bool, error) {
	var r rawAlbumList
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, false, fmt.Errorf("解析专辑列表失败: %w", err)
	}
	return MapAlbums(r.Albums), r.More, nil
}
