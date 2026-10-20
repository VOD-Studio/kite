// Package playlist 的浏览与发现接口声明。
//
// HighQuality/HighQualityTags/CatList/BrowseHot/Subscribers 五个读接口 AuthAnonymous + 24h 缓存
// (歌单浏览低频变化);TrackIDs 供 compose.PlaylistTracks 分页取歌曲,可选登录、不缓存。
package playlist

import (
	"encoding/json"
	"fmt"
	"time"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/model"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// defaultHighQualityCat 是精品歌单未指定分类时的默认分类。
// 上游对空 cat 返回「没有获取数据」(空列表);CLI 的 --cat 默认值也是它。gRPC 调用方省略可选字段
// (Rust 等客户端很常见)时若不兜底,就会拿到一个看似成功的空列表。
const defaultHighQualityCat = "全部"

// highQualityCat 返回请求的分类,未指定时用默认分类。
func highQualityCat(r *mmpb.HighQualityRequest) string {
	if r.GetCat() == "" {
		return defaultHighQualityCat
	}
	return r.GetCat()
}

// HighQuality 获取精品歌单列表。
var HighQuality = &engine.Endpoint[*mmpb.HighQualityRequest, *mmpb.HighQualityResponse]{
	Meta: weapiMeta("/weapi/playlist/highquality/list"),
	Cache: browseCache("playlist:highquality", func(r *mmpb.HighQualityRequest) string {
		return fmt.Sprintf("%s:%s:%d:%d", "playlist:highquality", highQualityCat(r), r.GetLimit(), r.GetOffset())
	}),
	NewResp: func() *mmpb.HighQualityResponse { return &mmpb.HighQualityResponse{} },
	MapRequest: func(req *mmpb.HighQualityRequest) (map[string]any, error) {
		limit := req.GetLimit()
		if limit <= 0 {
			limit = 20
		}
		return map[string]any{"cat": highQualityCat(req), "limit": limit, "offset": req.GetOffset()}, nil
	},
	MapResponse: parsePlaylistListResponse,
}

// HighQualityTags 获取精品歌单标签列表。
var HighQualityTags = &engine.Endpoint[*mmpb.HighQualityTagsRequest, *mmpb.HighQualityTagsResponse]{
	Meta:       weapiMeta("/weapi/playlist/highquality/tags"),
	Cache:      browseCacheConst[*mmpb.HighQualityTagsRequest]("playlist:hq:tags"),
	NewResp:    func() *mmpb.HighQualityTagsResponse { return &mmpb.HighQualityTagsResponse{} },
	MapRequest: func(*mmpb.HighQualityTagsRequest) (map[string]any, error) { return map[string]any{}, nil },
	MapResponse: func(_ *mmpb.HighQualityTagsRequest, raw json.RawMessage) (*mmpb.HighQualityTagsResponse, error) {
		// 真机返回的 category 是数字(如 0),旧 fixture 按字符串假设,两种形态都兼容。
		var resp struct {
			Tags []struct {
				Name     string          `json:"name"`     // 标签名
				Category json.RawMessage `json:"category"` // 分类(数字或字符串)
			} `json:"tags"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("解析精品标签失败: %w", err)
		}
		out := &mmpb.HighQualityTagsResponse{}
		for _, t := range resp.Tags {
			out.Tags = append(out.Tags, &mmpb.HighQualityTag{Name: t.Name, Category: rawToString(t.Category)})
		}
		return out, nil
	},
}

// rawToString 把 JSON 原始值转字符串:字符串去引号,数字等其他类型直接用原始字面量。
func rawToString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

// CatList 获取歌单分类列表。
var CatList = &engine.Endpoint[*mmpb.CatListRequest, *mmpb.CatListResponse]{
	Meta:       weapiMeta("/weapi/playlist/catalogue"),
	Cache:      browseCacheConst[*mmpb.CatListRequest]("playlist:catlist"),
	NewResp:    func() *mmpb.CatListResponse { return &mmpb.CatListResponse{} },
	MapRequest: func(*mmpb.CatListRequest) (map[string]any, error) { return map[string]any{}, nil },
	MapResponse: func(_ *mmpb.CatListRequest, raw json.RawMessage) (*mmpb.CatListResponse, error) {
		var resp struct {
			Sub []struct {
				Name          string `json:"name"`          // 分类名
				ResourceCount int64  `json:"resourceCount"` // 资源数
				Type          int64  `json:"type"`          // 类型ID
			} `json:"sub"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("解析歌单分类失败: %w", err)
		}
		out := &mmpb.CatListResponse{}
		for _, c := range resp.Sub {
			out.Categories = append(out.Categories, &mmpb.PlaylistCategory{
				Name: c.Name, ResourceCount: c.ResourceCount, TypeId: c.Type,
			})
		}
		return out, nil
	},
}

// BrowseHot 获取网友精选碟（热门歌单）。
var BrowseHot = &engine.Endpoint[*mmpb.BrowseHotRequest, *mmpb.BrowseHotResponse]{
	Meta: weapiMeta("/weapi/playlist/list"),
	Cache: browseCache("playlist:hot", func(r *mmpb.BrowseHotRequest) string {
		return fmt.Sprintf("%s:%s:%s:%d:%d", "playlist:hot", r.GetCat(), r.GetOrder(), r.GetLimit(), r.GetOffset())
	}),
	NewResp: func() *mmpb.BrowseHotResponse { return &mmpb.BrowseHotResponse{} },
	MapRequest: func(req *mmpb.BrowseHotRequest) (map[string]any, error) {
		limit := req.GetLimit()
		if limit <= 0 {
			limit = 30
		}
		order := req.GetOrder()
		if order == "" {
			order = "hot"
		}
		return map[string]any{"cat": req.GetCat(), "order": order, "limit": limit, "offset": req.GetOffset()}, nil
	},
	MapResponse: parseHotPlaylistResponse,
}

// Subscribers 获取歌单收藏者列表。
var Subscribers = &engine.Endpoint[*mmpb.SubscribersRequest, *mmpb.SubscribersResponse]{
	Meta: weapiMeta("/weapi/playlist/subscribers"),
	Cache: &engine.CachePolicy[*mmpb.SubscribersRequest]{
		Key: func(r *mmpb.SubscribersRequest) string {
			return fmt.Sprintf("playlist:subscribers:%d:%d:%d", r.GetPlaylistId(), r.GetLimit(), r.GetOffset())
		},
		TTL: 24 * time.Hour,
	},
	NewResp: func() *mmpb.SubscribersResponse { return &mmpb.SubscribersResponse{} },
	MapRequest: func(req *mmpb.SubscribersRequest) (map[string]any, error) {
		limit := req.GetLimit()
		if limit <= 0 {
			limit = 30
		}
		return map[string]any{"id": fmt.Sprintf("%d", req.GetPlaylistId()), "limit": limit, "offset": req.GetOffset()}, nil
	},
	MapResponse: func(_ *mmpb.SubscribersRequest, raw json.RawMessage) (*mmpb.SubscribersResponse, error) {
		var resp struct {
			Subscribers []struct {
				UserID    int64  `json:"userId"`    // 用户ID
				Nickname  string `json:"nickname"`  // 昵称
				AvatarURL string `json:"avatarUrl"` // 头像URL
			} `json:"subscribers"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("解析收藏者失败: %w", err)
		}
		out := &mmpb.SubscribersResponse{Total: int32(len(resp.Subscribers))}
		for _, s := range resp.Subscribers {
			out.Subscribers = append(out.Subscribers, &mmpb.User{
				Id: s.UserID, Nickname: s.Nickname, AvatarUrl: s.AvatarURL,
			})
		}
		return out, nil
	},
}

// TrackIDs 取歌单完整的曲目 ID 列表(不缓存)。
//
// 不直接对外:详情内嵌的 tracks 会被上游截断,按页取歌曲要用完整的 trackIds 切片,
// 再批量查这一页的歌曲详情,见 compose.PlaylistTracks(对外的 AllTracks)。
//
// AuthOptional:调用方带了网易云 cookie 就用它读(私有歌单),没带就匿名,不借用会话池。
var TrackIDs = &engine.Endpoint[*mmpb.GetPlaylistRequest, *model.PlaylistTrackIDs]{
	Meta: engine.Meta{
		Path: "/weapi/v6/playlist/detail", Method: "POST",
		Crypto: engine.CryptoWeAPI, Auth: session.AuthOptional,
	},
	MapRequest: func(req *mmpb.GetPlaylistRequest) (map[string]any, error) {
		// n 只影响内嵌 tracks 的数量,trackIds 总是完整的;取最少的内嵌歌曲以减小响应。
		return map[string]any{"id": fmt.Sprintf("%d", req.GetPlaylistId()), "n": 1, "s": 0}, nil
	},
	MapResponse: func(_ *mmpb.GetPlaylistRequest, raw json.RawMessage) (*model.PlaylistTrackIDs, error) {
		return model.DecodePlaylistTrackIDs(raw)
	},
}
