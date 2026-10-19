// Package artist 定义歌手接口的声明。
package artist

import (
	"encoding/json"
	"fmt"
	"time"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/model"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// GetArtist 是获取歌手信息及热门歌曲的接口声明。
//
// id 必须拼进 path(/weapi/v1/artist/{id})。曾用 /weapi/artist/head/info/get 且 id 放 body:
// 该接口响应形状是 data.artist(与本解码器期望的顶层 artist + hotSongs 不符),
// 宽松解析下静默得到空歌手——RPC 成功但返回空。
var GetArtist = &engine.Endpoint[*mmpb.GetArtistRequest, *mmpb.GetArtistResponse]{
	Meta: engine.Meta{
		Path:   "/weapi/v1/artist", // 占位,实际 path 由 PathFunc 生成
		Method: "POST",
		Crypto: engine.CryptoWeAPI,
		Auth:   session.AuthAnonymous,
	},
	PathFunc: func(req *mmpb.GetArtistRequest) string {
		return fmt.Sprintf("/weapi/v1/artist/%d", req.GetArtistId())
	},
	Cache: &engine.CachePolicy[*mmpb.GetArtistRequest]{
		Key: func(req *mmpb.GetArtistRequest) string {
			return fmt.Sprintf("artist:info:%d", req.GetArtistId())
		},
		TTL: 24 * time.Hour,
	},
	NewResp: func() *mmpb.GetArtistResponse { return &mmpb.GetArtistResponse{} },
	MapRequest: func(_ *mmpb.GetArtistRequest) (map[string]any, error) {
		return map[string]any{"top": 50, "offset": 0}, nil
	},
	MapResponse: func(_ *mmpb.GetArtistRequest, raw json.RawMessage) (*mmpb.GetArtistResponse, error) {
		a, songs, err := model.DecodeArtistInfo(raw)
		if err != nil {
			return &mmpb.GetArtistResponse{}, err
		}
		return &mmpb.GetArtistResponse{Artist: a, HotSongs: songs}, nil
	},
}
