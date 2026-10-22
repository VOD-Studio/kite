// Package playlist 的写操作接口声明。
//
// 7 个写操作（Subscribe/Create/Delete/UpdateName/UpdateDesc/UpdateTags/UpdateTracks）。
// 歌单排序（UpdateOrder）和歌曲排序（UpdateSongOrder）的网易云端点不稳定，推迟验证。
// 全部 Cache=nil（写操作不缓存）、AuthLoggedIn（需登录态）。
// cookie 经 metadata "x-netease-cookie" 传入（interceptor 注入 context），
// 与 CLI 一样走 ctx cookie 路径（engine.ExecuteWithCookie），不走 session 池。
//
// 与其它域一致，都是声明式 engine.Endpoint：service 与 CLI 各一行调用，参数构造与响应解析只在这里写一份。
package playlist

import (
	"encoding/json"
	"fmt"
	"strings"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// writeMeta 构造 weapi POST + AuthLoggedIn 的 Meta（歌单写操作统一参数）。
func writeMeta(path string) engine.Meta {
	return engine.Meta{Path: path, Method: "POST", Crypto: engine.CryptoWeAPI, Auth: session.AuthLoggedIn}
}

// Subscribe 收藏/取消收藏歌单（返回操作后是否已收藏）。
var Subscribe = &engine.Endpoint[*mmpb.SubscribeRequest, *mmpb.SubscribeResponse]{
	Meta:    writeMeta("/weapi/playlist/subscribe"),
	NewResp: func() *mmpb.SubscribeResponse { return &mmpb.SubscribeResponse{} },
	MapRequest: func(req *mmpb.SubscribeRequest) (map[string]any, error) {
		return map[string]any{"id": fmt.Sprintf("%d", req.GetPlaylistId())}, nil
	},
	MapResponse: func(_ *mmpb.SubscribeRequest, raw json.RawMessage) (*mmpb.SubscribeResponse, error) {
		resp := engine.DecodeLoose[struct {
			Subscribed bool `json:"subscribed"` // 操作后是否已收藏
		}](raw) // 写操作响应宽松解析:缺字段按零值
		return &mmpb.SubscribeResponse{Subscribed: resp.Subscribed}, nil
	},
}

// Create 新建歌单（返回歌单 ID）。
var Create = &engine.Endpoint[*mmpb.CreateRequest, *mmpb.CreateResponse]{
	Meta:    writeMeta("/weapi/playlist/create"),
	NewResp: func() *mmpb.CreateResponse { return &mmpb.CreateResponse{} },
	MapRequest: func(req *mmpb.CreateRequest) (map[string]any, error) {
		return map[string]any{
			"name":    req.GetName(),
			"privacy": boolToInt(req.GetPrivacy()),
		}, nil
	},
	MapResponse: func(_ *mmpb.CreateRequest, raw json.RawMessage) (*mmpb.CreateResponse, error) {
		resp := engine.DecodeLoose[struct {
			ID       int64 `json:"id"` // 新建歌单ID（部分版本在 playlist.id）
			Playlist struct {
				ID int64 `json:"id"` // 歌单ID
			} `json:"playlist"` // 歌单信息
		}](raw)
		id := resp.ID
		if id == 0 {
			id = resp.Playlist.ID
		}
		return &mmpb.CreateResponse{PlaylistId: id}, nil
	},
}

// Delete 删除歌单。
var Delete = &engine.Endpoint[*mmpb.DeleteRequest, *mmpb.DeleteResponse]{
	Meta:    writeMeta("/weapi/playlist/delete"),
	NewResp: func() *mmpb.DeleteResponse { return &mmpb.DeleteResponse{} },
	MapRequest: func(req *mmpb.DeleteRequest) (map[string]any, error) {
		return map[string]any{"ids": fmt.Sprintf("[%d]", req.GetPlaylistId())}, nil
	},
	MapResponse: func(_ *mmpb.DeleteRequest, _ json.RawMessage) (*mmpb.DeleteResponse, error) {
		return &mmpb.DeleteResponse{}, nil
	},
}

// UpdateName 更新歌单名。
var UpdateName = &engine.Endpoint[*mmpb.UpdateNameRequest, *mmpb.UpdateNameResponse]{
	Meta:    writeMeta("/weapi/playlist/update/name"),
	NewResp: func() *mmpb.UpdateNameResponse { return &mmpb.UpdateNameResponse{} },
	MapRequest: func(req *mmpb.UpdateNameRequest) (map[string]any, error) {
		return map[string]any{"id": fmt.Sprintf("%d", req.GetPlaylistId()), "name": req.GetName()}, nil
	},
	MapResponse: func(_ *mmpb.UpdateNameRequest, _ json.RawMessage) (*mmpb.UpdateNameResponse, error) {
		return &mmpb.UpdateNameResponse{}, nil
	},
}

// UpdateDesc 更新歌单描述。
var UpdateDesc = &engine.Endpoint[*mmpb.UpdateDescRequest, *mmpb.UpdateDescResponse]{
	Meta:    writeMeta("/weapi/playlist/update/description"),
	NewResp: func() *mmpb.UpdateDescResponse { return &mmpb.UpdateDescResponse{} },
	MapRequest: func(req *mmpb.UpdateDescRequest) (map[string]any, error) {
		return map[string]any{"id": fmt.Sprintf("%d", req.GetPlaylistId()), "desc": req.GetDesc()}, nil
	},
	MapResponse: func(_ *mmpb.UpdateDescRequest, _ json.RawMessage) (*mmpb.UpdateDescResponse, error) {
		return &mmpb.UpdateDescResponse{}, nil
	},
}

// UpdateTags 更新歌单标签。
var UpdateTags = &engine.Endpoint[*mmpb.UpdateTagsRequest, *mmpb.UpdateTagsResponse]{
	Meta:    writeMeta("/weapi/playlist/update/tags"),
	NewResp: func() *mmpb.UpdateTagsResponse { return &mmpb.UpdateTagsResponse{} },
	MapRequest: func(req *mmpb.UpdateTagsRequest) (map[string]any, error) {
		return map[string]any{"id": fmt.Sprintf("%d", req.GetPlaylistId()), "tags": req.GetTags()}, nil
	},
	MapResponse: func(_ *mmpb.UpdateTagsRequest, _ json.RawMessage) (*mmpb.UpdateTagsResponse, error) {
		return &mmpb.UpdateTagsResponse{}, nil
	},
}

// UpdateTracks 添加或删除歌曲。op=add / del，trackIds 逗号分隔。
var UpdateTracks = &engine.Endpoint[*mmpb.UpdateTracksRequest, *mmpb.UpdateTracksResponse]{
	Meta:    writeMeta("/weapi/playlist/manipulate/tracks"),
	NewResp: func() *mmpb.UpdateTracksResponse { return &mmpb.UpdateTracksResponse{} },
	MapRequest: func(req *mmpb.UpdateTracksRequest) (map[string]any, error) {
		op := "add"
		if req.GetOp() == mmpb.TracksOp_TRACKS_OP_DEL {
			op = "del"
		}
		ids := make([]string, 0, len(req.GetTrackIds()))
		for _, id := range req.GetTrackIds() {
			ids = append(ids, fmt.Sprintf("%d", id))
		}
		return map[string]any{
			"op":     op,
			"pid":    fmt.Sprintf("%d", req.GetPlaylistId()),
			"tracks": strings.Join(ids, ","),
		}, nil
	},
	MapResponse: func(_ *mmpb.UpdateTracksRequest, raw json.RawMessage) (*mmpb.UpdateTracksResponse, error) {
		resp := engine.DecodeLoose[struct {
			Body []struct {
				TrackID int64 `json:"trackId"` // 歌曲ID
			} `json:"body"` // 成功操作的歌曲
		}](raw)
		out := &mmpb.UpdateTracksResponse{}
		for _, b := range resp.Body {
			out.BodyTrackIds = append(out.BodyTrackIds, b.TrackID)
		}
		return out, nil
	},
}

// boolToInt 把 bool 转成 0/1（网易云用整数表示布尔）。
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
