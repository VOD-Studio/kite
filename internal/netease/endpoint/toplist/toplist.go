// Package toplist 定义官方榜单接口的声明。
//
// 榜单即歌单:Toplist.id 可直接用于歌单接口读完整曲目;完整榜单歌曲分页
// 依赖跨接口组合层(internal/netease/compose,PR #94),该层就绪前由
// CLI 经歌单域命令(kite playlist detail/tracks)承接。
package toplist

import (
	"encoding/json"
	"time"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/model"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// List 获取全部官方榜单列表(/toplist,64 个榜单,不含歌曲概要)。
var List = &engine.Endpoint[*mmpb.ToplistsRequest, *mmpb.ToplistsResponse]{
	Meta: engine.Meta{
		Path:   "/weapi/toplist",
		Method: "POST",
		Crypto: engine.CryptoWeAPI,
		Auth:   session.AuthAnonymous,
	},
	Cache: &engine.CachePolicy[*mmpb.ToplistsRequest]{
		Key: func(*mmpb.ToplistsRequest) string { return "toplist:list" },
		TTL: time.Hour, // 榜单目录变化低频,主榜曲目每小时更新
	},
	NewResp:    func() *mmpb.ToplistsResponse { return &mmpb.ToplistsResponse{} },
	MapRequest: func(*mmpb.ToplistsRequest) (map[string]any, error) { return map[string]any{}, nil },
	MapResponse: func(_ *mmpb.ToplistsRequest, raw json.RawMessage) (*mmpb.ToplistsResponse, error) {
		toplists, err := model.DecodeToplists(raw)
		if err != nil {
			return &mmpb.ToplistsResponse{}, err
		}
		return &mmpb.ToplistsResponse{Toplists: toplists}, nil
	},
}

// Detail 获取全部官方榜单详情(/toplist/detail,四个主榜各带前三歌曲概要)。
var Detail = &engine.Endpoint[*mmpb.ToplistDetailRequest, *mmpb.ToplistDetailResponse]{
	Meta: engine.Meta{
		Path:   "/weapi/toplist/detail",
		Method: "POST",
		Crypto: engine.CryptoWeAPI,
		Auth:   session.AuthAnonymous,
	},
	Cache: &engine.CachePolicy[*mmpb.ToplistDetailRequest]{
		Key: func(*mmpb.ToplistDetailRequest) string { return "toplist:detail" },
		TTL: time.Hour,
	},
	NewResp:    func() *mmpb.ToplistDetailResponse { return &mmpb.ToplistDetailResponse{} },
	MapRequest: func(*mmpb.ToplistDetailRequest) (map[string]any, error) { return map[string]any{}, nil },
	MapResponse: func(_ *mmpb.ToplistDetailRequest, raw json.RawMessage) (*mmpb.ToplistDetailResponse, error) {
		toplists, err := model.DecodeToplists(raw)
		if err != nil {
			return &mmpb.ToplistDetailResponse{}, err
		}
		return &mmpb.ToplistDetailResponse{Toplists: toplists}, nil
	},
}
