// Package fm 定义私人电台接口的声明。
package fm

import (
	"encoding/json"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/model"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// GetPersonalFM 是获取私人 FM 歌曲的接口声明（需登录态）。
var GetPersonalFM = &engine.Endpoint[*mmpb.GetPersonalFMRequest, *mmpb.GetPersonalFMResponse]{
	Meta: engine.Meta{
		Path:   "/weapi/v1/radio/get",
		Method: "POST",
		Crypto: engine.CryptoWeAPI,
		Auth:   session.AuthLoggedIn,
	},
	// 不缓存：私人 FM 每次请求上游都会返回新的一批歌曲（客户端靠重复调用"取下一批"），
	// 缓存会让同一用户在 TTL 内反复拿到同一批。
	Cache:   nil,
	NewResp: func() *mmpb.GetPersonalFMResponse { return &mmpb.GetPersonalFMResponse{} },
	MapRequest: func(*mmpb.GetPersonalFMRequest) (map[string]any, error) {
		return map[string]any{}, nil
	},
	MapResponse: func(_ *mmpb.GetPersonalFMRequest, raw json.RawMessage) (*mmpb.GetPersonalFMResponse, error) {
		songs, err := model.DecodePersonalFM(raw)
		if err != nil {
			return &mmpb.GetPersonalFMResponse{}, err
		}
		return &mmpb.GetPersonalFMResponse{Songs: songs}, nil
	},
}
