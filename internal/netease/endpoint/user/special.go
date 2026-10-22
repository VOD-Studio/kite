// Package user 的特殊接口声明（不走标准 Execute 缓存路径）。
//
// DetailByName/FollowEachOther 是匿名 cookie 的直接调用,都是声明式 engine.Endpoint,
// service 层走 ctx cookie 路径(engine.ExecuteWithCookie)。DetailByName 用公开搜索实现;
// FollowEachOther 需要动态 path(uid 拼进 URL,用 PathFunc)且要遍历关注列表做业务判断。
// 两者不缓存（动态/即时数据）。
package user

import (
	"encoding/json"
	"fmt"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// DetailByName 根据 nickname 获取 userid（匿名 cookie）。
//
// 上游没有昵称直查接口（旧 path /weapi/v1/w/user/info/detail 真机返回 404),
// 改用公开搜索 type=1002(用户)取第一条结果。
var DetailByName = &engine.Endpoint[*mmpb.DetailByNameRequest, *mmpb.DetailByNameResponse]{
	Meta: engine.Meta{
		Path:   "/api/search/get",
		Method: "GET",
		Crypto: engine.CryptoNone,
		Auth:   session.AuthAnonymous,
	},
	MapRequest: func(req *mmpb.DetailByNameRequest) (map[string]any, error) {
		return map[string]any{
			"s":      req.GetNickname(),
			"type":   1002, // 搜索用户
			"limit":  1,
			"offset": 0,
		}, nil
	},
	MapResponse: func(_ *mmpb.DetailByNameRequest, raw json.RawMessage) (*mmpb.DetailByNameResponse, error) {
		resp, err := engine.Decode[struct {
			Result struct {
				Userprofiles []struct {
					UserID int64 `json:"userId"` // 用户ID
				} `json:"userprofiles"`
			} `json:"result"`
		}](raw, "昵称查询")
		if err != nil {
			return nil, err
		}
		if len(resp.Result.Userprofiles) == 0 {
			return nil, fmt.Errorf("未找到该昵称对应的用户: %w", merrors.ErrNotFound)
		}
		return &mmpb.DetailByNameResponse{UserId: resp.Result.Userprofiles[0].UserID}, nil
	},
}

// FollowEachOther 判断两个用户是否互相关注（查 target 的关注列表里是否含 user）。
//
// 网易云无直接接口，取 target 的关注列表遍历判断。path 需要把 target uid 拼进 URL
// （/weapi/user/getfollows/{uid}），用 PathFunc 动态生成。不缓存（即时数据）。
var FollowEachOther = &engine.Endpoint[*mmpb.FollowEachOtherRequest, *mmpb.FollowEachOtherResponse]{
	Meta: engine.Meta{
		Path:   "/weapi/user/getfollows", // 占位，实际 path 由 PathFunc 生成
		Method: "POST",
		Crypto: engine.CryptoWeAPI,
		Auth:   session.AuthAnonymous,
	},
	PathFunc: func(req *mmpb.FollowEachOtherRequest) string {
		return fmt.Sprintf("/weapi/user/getfollows/%d", req.GetTargetUserId())
	},
	NewResp: func() *mmpb.FollowEachOtherResponse { return &mmpb.FollowEachOtherResponse{} },
	// 最多拉前 100 条做包含判断。
	MapRequest: func(_ *mmpb.FollowEachOtherRequest) (map[string]any, error) {
		return map[string]any{"offset": 0, "limit": 100, "order": true}, nil
	},
	// 遍历响应的 follow 数组,判断是否包含 req.UserId。
	MapResponse: func(req *mmpb.FollowEachOtherRequest, raw json.RawMessage) (*mmpb.FollowEachOtherResponse, error) {
		resp, err := engine.Decode[struct {
			Follow []struct {
				UserID int64 `json:"userId"` // 被关注用户ID
			} `json:"follow"`
		}](raw, "关注列表")
		if err != nil {
			return nil, err
		}
		for _, f := range resp.Follow {
			if f.UserID == req.GetUserId() {
				return &mmpb.FollowEachOtherResponse{FollowEachOther: true}, nil
			}
		}
		return &mmpb.FollowEachOtherResponse{FollowEachOther: false}, nil
	},
}
