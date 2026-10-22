// Package mv 的列表类接口声明(排行 / 相关 / 收藏)。
package mv

import (
	"encoding/json"
	"fmt"
	"time"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/model"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// Top 获取 MV 排行榜(/mv/toplist,现行接口;老 /top/mv 已业务 404)。
var Top = &engine.Endpoint[*mmpb.TopMvsRequest, *mmpb.TopMvsResponse]{
	Meta: engine.Meta{
		Path:   "/weapi/mv/toplist",
		Method: "POST",
		Crypto: engine.CryptoWeAPI,
		Auth:   session.AuthAnonymous,
	},
	Cache: &engine.CachePolicy[*mmpb.TopMvsRequest]{
		Key: func(*mmpb.TopMvsRequest) string { return "mv:top" },
		TTL: time.Hour, // 榜单按小时级更新(响应带 updateTime)
	},
	NewResp:    func() *mmpb.TopMvsResponse { return &mmpb.TopMvsResponse{} },
	MapRequest: func(*mmpb.TopMvsRequest) (map[string]any, error) { return map[string]any{}, nil },
	MapResponse: func(_ *mmpb.TopMvsRequest, raw json.RawMessage) (*mmpb.TopMvsResponse, error) {
		mvs, hasMore, updateTime, err := model.DecodeTopMvs(raw)
		if err != nil {
			return &mmpb.TopMvsResponse{}, err
		}
		return &mmpb.TopMvsResponse{Mvs: mvs, HasMore: hasMore, UpdateTimeMs: updateTime}, nil
	},
}

// Similar 获取相关 MV(/simi/mv)。
var Similar = &engine.Endpoint[*mmpb.SimilarMvsRequest, *mmpb.SimilarMvsResponse]{
	Meta: engine.Meta{
		Path:   "/eapi/discovery/simiMV",
		Method: "POST",
		Crypto: engine.CryptoEAPI,
		Auth:   session.AuthAnonymous,
	},
	Cache: &engine.CachePolicy[*mmpb.SimilarMvsRequest]{
		Key: func(req *mmpb.SimilarMvsRequest) string { return fmt.Sprintf("mv:similar:%d", req.GetMvId()) },
		TTL: 24 * time.Hour,
	},
	NewResp: func() *mmpb.SimilarMvsResponse { return &mmpb.SimilarMvsResponse{} },
	MapRequest: func(req *mmpb.SimilarMvsRequest) (map[string]any, error) {
		return map[string]any{"mvid": fmt.Sprintf("%d", req.GetMvId())}, nil
	},
	MapResponse: func(_ *mmpb.SimilarMvsRequest, raw json.RawMessage) (*mmpb.SimilarMvsResponse, error) {
		mvs, _, err := model.DecodeMvSummaries(raw, "mvs")
		if err != nil {
			return &mmpb.SimilarMvsResponse{}, err
		}
		return &mmpb.SimilarMvsResponse{Mvs: mvs}, nil
	},
}

// Subscribed 获取收藏的 MV 列表(/mv/sublist,需登录态)。
// 匿名调用上游不报错而是返回空列表,登录后才有数据,故声明 AuthLoggedIn
// 由调用方保证凭据;账号私有数据不缓存。
var Subscribed = &engine.Endpoint[*mmpb.SubscribedMvsRequest, *mmpb.SubscribedMvsResponse]{
	Meta: engine.Meta{
		Path:   "/weapi/mv/sublist",
		Method: "POST",
		Crypto: engine.CryptoWeAPI,
		Auth:   session.AuthLoggedIn,
	},
	Cache:   nil,
	NewResp: func() *mmpb.SubscribedMvsResponse { return &mmpb.SubscribedMvsResponse{} },
	MapRequest: func(req *mmpb.SubscribedMvsRequest) (map[string]any, error) {
		limit := req.GetLimit()
		if limit <= 0 {
			limit = 25
		}
		return map[string]any{"limit": limit, "offset": req.GetOffset(), "total": "true"}, nil
	},
	MapResponse: func(_ *mmpb.SubscribedMvsRequest, raw json.RawMessage) (*mmpb.SubscribedMvsResponse, error) {
		mvs, hasMore, err := model.DecodeMvSummaries(raw, "data")
		if err != nil {
			return &mmpb.SubscribedMvsResponse{}, err
		}
		return &mmpb.SubscribedMvsResponse{Mvs: mvs, HasMore: hasMore}, nil
	},
}
