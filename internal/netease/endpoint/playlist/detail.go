// Package playlist 定义歌单接口的声明。
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

// GetPlaylist 是获取歌单详情的接口声明。
//
// AuthOptional:调用方带了网易云 cookie 就用它读(私有歌单),没带就匿名,不借用会话池;
// 带 cookie 的结果按凭据指纹单独缓存,不同账号与匿名请求互不命中。
var GetPlaylist = &engine.Endpoint[*mmpb.GetPlaylistRequest, *mmpb.GetPlaylistResponse]{
	Meta: engine.Meta{
		Path:   "/weapi/v6/playlist/detail",
		Method: "POST",
		Crypto: engine.CryptoWeAPI,
		Auth:   session.AuthOptional,
	},
	Cache: &engine.CachePolicy[*mmpb.GetPlaylistRequest]{
		Key: func(req *mmpb.GetPlaylistRequest) string {
			return fmt.Sprintf("playlist:detail:%d", req.GetPlaylistId())
		},
		TTL: 24 * time.Hour,
	},
	NewResp: func() *mmpb.GetPlaylistResponse { return &mmpb.GetPlaylistResponse{} },
	MapRequest: func(req *mmpb.GetPlaylistRequest) (map[string]any, error) {
		return map[string]any{
			"id": fmt.Sprintf("%d", req.GetPlaylistId()),
			"n":  1000,
			"s":  8,
		}, nil
	},
	MapResponse: func(_ *mmpb.GetPlaylistRequest, raw json.RawMessage) (*mmpb.GetPlaylistResponse, error) {
		pl, err := model.MapPlaylist(raw)
		if err != nil {
			return &mmpb.GetPlaylistResponse{}, err
		}
		return &mmpb.GetPlaylistResponse{Playlist: pl}, nil
	},
}
