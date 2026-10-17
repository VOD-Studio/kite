// Package artist 的收藏写操作接口声明。
package artist

import (
	"encoding/json"
	"fmt"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// Subscribe 收藏/取消收藏歌手（写操作，ctx cookie 路径，不缓存）。
var Subscribe = &engine.Endpoint[*mmpb.ArtistSubscribeRequest, *mmpb.ArtistSubscribeResponse]{
	Meta: engine.Meta{
		Path: "/weapi/artist/sub", Method: "POST",
		Crypto: engine.CryptoWeAPI, Auth: session.AuthLoggedIn,
	},
	NewResp: func() *mmpb.ArtistSubscribeResponse { return &mmpb.ArtistSubscribeResponse{} },
	MapRequest: func(req *mmpb.ArtistSubscribeRequest) (map[string]any, error) {
		return map[string]any{
			"artistId":  fmt.Sprintf("%d", req.GetArtistId()),
			"artistIds": fmt.Sprintf("[%d]", req.GetArtistId()),
		}, nil
	},
	MapResponse: func(_ *mmpb.ArtistSubscribeRequest, raw json.RawMessage) (*mmpb.ArtistSubscribeResponse, error) {
		var resp struct {
			Sub bool `json:"sub"` // 操作后是否已收藏
		}
		_ = json.Unmarshal(raw, &resp) // 写操作响应宽松解析:缺字段按零值
		return &mmpb.ArtistSubscribeResponse{Subscribed: resp.Sub}, nil
	},
}
