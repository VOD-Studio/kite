// Package djradio 声明主播电台域接口(issue #33 第一阶段:只读)。
//
// 第一阶段只有 endpoint 声明 + CLI 直连验证,请求 / 响应都是纯 Go 类型
// (请求在本包,响应在 model),proto 建模在第二阶段。因此不声明缓存:
// engine 的缓存序列化走 proto.Marshal,非 proto 响应存不进去,等第二阶段再补。
package djradio

import (
	"encoding/json"
	"fmt"

	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/model"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// CateListRequest 电台分类列表请求(无参数)。
type CateListRequest struct{}

// ProgramsRequest 电台节目列表请求(分页)。
type ProgramsRequest struct {
	RadioID int64 // 电台 ID
	Limit   int32 // 每页数量,<=0 时上游默认 30
	Offset  int32 // 偏移量
	Asc     bool  // true=按上架时间升序(默认降序,最新在前)
}

// ProgramDetailRequest 节目详情请求。
type ProgramDetailRequest struct {
	ProgramID int64 // 节目 ID
}

// SubscribedRequest 订阅电台列表请求(分页,查当前登录账号)。
type SubscribedRequest struct {
	Limit  int32 // 每页数量,<=0 时上游默认 30
	Offset int32 // 偏移量
}

// weapiMeta 构造 weapi POST 的 Meta(auth 按接口登录态需求传入)。
func weapiMeta(path string, auth session.AuthRequirement) engine.Meta {
	return engine.Meta{Path: path, Method: "POST", Crypto: engine.CryptoWeAPI, Auth: auth}
}

// CateList 获取电台分类列表(公开数据,匿名可用)。
var CateList = &engine.Endpoint[*CateListRequest, *model.DjCategoryList]{
	Meta:       weapiMeta("/weapi/djradio/category/get", session.AuthAnonymous),
	MapRequest: func(*CateListRequest) (map[string]any, error) { return map[string]any{}, nil },
	MapResponse: func(_ *CateListRequest, raw json.RawMessage) (*model.DjCategoryList, error) {
		return model.DecodeDjCategories(raw)
	},
}

// Programs 获取电台节目列表(分页,公开数据,匿名可用)。
var Programs = &engine.Endpoint[*ProgramsRequest, *model.DjProgramPage]{
	Meta: weapiMeta("/weapi/dj/program/byradio", session.AuthAnonymous),
	MapRequest: func(req *ProgramsRequest) (map[string]any, error) {
		limit := req.Limit
		if limit <= 0 {
			limit = 30
		}
		return map[string]any{
			"radioId": fmt.Sprintf("%d", req.RadioID),
			"limit":   limit, "offset": req.Offset, "asc": req.Asc,
		}, nil
	},
	MapResponse: func(_ *ProgramsRequest, raw json.RawMessage) (*model.DjProgramPage, error) {
		return model.DecodeDjPrograms(raw)
	},
}

// ProgramDetail 获取节目详情(公开数据,匿名可用)。
var ProgramDetail = &engine.Endpoint[*ProgramDetailRequest, *model.DjProgram]{
	Meta: weapiMeta("/weapi/dj/program/detail", session.AuthAnonymous),
	MapRequest: func(req *ProgramDetailRequest) (map[string]any, error) {
		return map[string]any{"id": fmt.Sprintf("%d", req.ProgramID)}, nil
	},
	MapResponse: func(_ *ProgramDetailRequest, raw json.RawMessage) (*model.DjProgram, error) {
		return model.DecodeDjProgramDetail(raw)
	},
}

// Subscribed 获取当前登录账号订阅的电台列表(需登录态)。
//
// 匿名调用上游不报 301 而是返回 200 + 空 djRadios(2026-10 实测),
// 「看似成功实则为空」正是它要登录态的信号;CLI 侧先 RequireLogin 再调。
var Subscribed = &engine.Endpoint[*SubscribedRequest, *model.DjSubscribedPage]{
	Meta: weapiMeta("/weapi/djradio/get/subed", session.AuthLoggedIn),
	MapRequest: func(req *SubscribedRequest) (map[string]any, error) {
		limit := req.Limit
		if limit <= 0 {
			limit = 30
		}
		return map[string]any{"limit": limit, "offset": req.Offset, "total": true}, nil
	},
	MapResponse: func(_ *SubscribedRequest, raw json.RawMessage) (*model.DjSubscribedPage, error) {
		return model.DecodeDjSubscribed(raw)
	},
}
