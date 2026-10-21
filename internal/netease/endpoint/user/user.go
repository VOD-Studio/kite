// Package user 定义用户模块接口的声明。
package user

import (
	"encoding/json"
	"fmt"
	"time"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/model"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// Account 是获取当前登录账号信息的接口声明（需登录态）。
var Account = &engine.Endpoint[*mmpb.AccountRequest, *mmpb.AccountResponse]{
	Meta: engine.Meta{
		Path: "/weapi/w/nuser/account/get", Method: "POST",
		Crypto: engine.CryptoWeAPI, Auth: session.AuthLoggedIn,
	},
	NewResp:    func() *mmpb.AccountResponse { return &mmpb.AccountResponse{} },
	MapRequest: func(*mmpb.AccountRequest) (map[string]any, error) { return map[string]any{}, nil },
	MapResponse: func(_ *mmpb.AccountRequest, raw json.RawMessage) (*mmpb.AccountResponse, error) {
		u, err := model.DecodeUserDetail(raw)
		if err != nil {
			return nil, err
		}
		return &mmpb.AccountResponse{User: u}, nil
	},
}

// Detail 是获取用户详情的接口声明。
var Detail = &engine.Endpoint[*mmpb.DetailRequest, *mmpb.DetailResponse]{
	Meta: engine.Meta{
		Path: "/weapi/v1/user/detail", Method: "POST", // 占位,实际 path 由 PathFunc 生成
		Crypto: engine.CryptoWeAPI, Auth: session.AuthAnonymous,
	},
	// uid 必须拼进 path。曾用 /weapi/v3/user/detail 且 id 放 body:真实上游返回空响应,
	// 服务端因此恒报 NOT_FOUND,而 CLI 绕过 endpoint 用的正是 path 形式。
	PathFunc: func(req *mmpb.DetailRequest) string {
		return fmt.Sprintf("/weapi/v1/user/detail/%d", req.GetUserId())
	},
	Cache: &engine.CachePolicy[*mmpb.DetailRequest]{
		Key: func(req *mmpb.DetailRequest) string { return fmt.Sprintf("user:detail:%d", req.GetUserId()) },
		TTL: 24 * time.Hour,
	},
	NewResp: func() *mmpb.DetailResponse { return &mmpb.DetailResponse{} },
	MapRequest: func(_ *mmpb.DetailRequest) (map[string]any, error) {
		return map[string]any{}, nil
	},
	MapResponse: func(_ *mmpb.DetailRequest, raw json.RawMessage) (*mmpb.DetailResponse, error) {
		u, err := model.DecodeUserDetail(raw)
		if err != nil {
			return nil, err
		}
		return &mmpb.DetailResponse{User: u}, nil
	},
}

// SubCount 是获取用户数量统计的接口声明。
var SubCount = &engine.Endpoint[*mmpb.SubCountRequest, *mmpb.SubCountResponse]{
	Meta: engine.Meta{
		Path: "/weapi/subcount", Method: "POST",
		Crypto: engine.CryptoWeAPI, Auth: session.AuthAnonymous,
	},
	Cache: &engine.CachePolicy[*mmpb.SubCountRequest]{
		Key: func(req *mmpb.SubCountRequest) string { return fmt.Sprintf("user:subcount:%d", req.GetUserId()) },
		TTL: 24 * time.Hour,
	},
	NewResp: func() *mmpb.SubCountResponse { return &mmpb.SubCountResponse{} },
	MapRequest: func(_ *mmpb.SubCountRequest) (map[string]any, error) {
		// SubCount 需登录态查自己，此处传 userId 给上游。
		return map[string]any{}, nil
	},
	MapResponse: func(_ *mmpb.SubCountRequest, raw json.RawMessage) (*mmpb.SubCountResponse, error) {
		c, err := model.DecodeUserSubCount(raw)
		if err != nil {
			return nil, err
		}
		return &mmpb.SubCountResponse{Count: c}, nil
	},
}

// PlaylistPage 取用户歌单的一页原始列表(不筛选、不缓存)。
//
// 不直接对外:上游第一页会在 limit 之外额外插入「喜欢的音乐」,也不返回总数,
// 创建 / 收藏又只能在拿到数据后判断。对外的 UserPlaylist 由 compose.UserPlaylists
// 取全量后再筛选、分页。
//
// AuthOptional:调用方带了网易云 cookie 就用它读(能看到本人的私有歌单),
// 没带就匿名;任何情况下都不借用会话池里保存的账号。
var PlaylistPage = &engine.Endpoint[*mmpb.UserPlaylistRequest, *model.UserPlaylistPage]{
	Meta: engine.Meta{
		Path: "/weapi/user/playlist", Method: "POST",
		Crypto: engine.CryptoWeAPI, Auth: session.AuthOptional,
	},
	MapRequest: func(req *mmpb.UserPlaylistRequest) (map[string]any, error) {
		limit := int(req.GetLimit())
		if limit <= 0 {
			limit = 30
		}
		return map[string]any{
			"uid":    fmt.Sprintf("%d", req.GetUserId()),
			"limit":  limit,
			"offset": req.GetOffset(),
		}, nil
	},
	MapResponse: func(_ *mmpb.UserPlaylistRequest, raw json.RawMessage) (*model.UserPlaylistPage, error) {
		return model.DecodeUserPlaylistPage(raw)
	},
}
