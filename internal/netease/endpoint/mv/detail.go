// Package mv 定义 MV 域接口的声明(issue #32 第一阶段:endpoint 声明 +
// CLI 直连验证,不经 gRPC / service)。
//
// 上游现状(2026-10 实测):独立的 /mv/url 接口已下线(weapi / eapi 各参数
// 形态均业务 404),多清晰度播放地址由 /mv/detail 的 brs 字段返回,
// CLI 的 `kite mv url` 即经 Detail 取;MV 排行的现行接口是 /mv/toplist
// (老 /top/mv 已业务 404)。
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

// Detail 获取 MV 详情(/mv/detail,brs 含多清晰度播放地址)。
var Detail = &engine.Endpoint[*mmpb.GetMvRequest, *mmpb.GetMvResponse]{
	Meta: engine.Meta{
		Path:   "/weapi/mv/detail",
		Method: "POST",
		Crypto: engine.CryptoWeAPI,
		Auth:   session.AuthAnonymous,
	},
	Cache: &engine.CachePolicy[*mmpb.GetMvRequest]{
		Key: func(req *mmpb.GetMvRequest) string { return fmt.Sprintf("mv:detail:%d", req.GetMvId()) },
		TTL: time.Hour, // brs 播放地址带签名有时效,playCount 也会更新,不缓存过久
	},
	NewResp: func() *mmpb.GetMvResponse { return &mmpb.GetMvResponse{} },
	MapRequest: func(req *mmpb.GetMvRequest) (map[string]any, error) {
		// 实测 body 参数名是 id(字符串):传 mvid 时上游 HTTP 400。
		return map[string]any{"id": fmt.Sprintf("%d", req.GetMvId())}, nil
	},
	MapResponse: func(_ *mmpb.GetMvRequest, raw json.RawMessage) (*mmpb.GetMvResponse, error) {
		mv, err := model.DecodeMvDetail(raw)
		if err != nil {
			return &mmpb.GetMvResponse{}, err
		}
		return &mmpb.GetMvResponse{Mv: mv}, nil
	},
}
