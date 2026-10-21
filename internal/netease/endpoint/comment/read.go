// Package comment 的读类接口声明:评论列表(v2)与热门评论。
package comment

import (
	"encoding/json"
	"fmt"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// readMeta 构造评论读接口的 Meta。
//
// AuthOptional:评论是公开数据,匿名可读;但 liked(是否已赞)是 per-user 的,
// 调用方经 ctx 带了 cookie 就用它(与 playlist 详情同一取舍),没带就匿名,
// 不借用会话池——engine.Execute 与 CLI 的 ExecuteWithCookie 凭据行为一致。
func readMeta(path string, crypto engine.CryptoMethod) engine.Meta {
	return engine.Meta{Path: path, Method: "POST", Crypto: crypto, Auth: session.AuthOptional}
}

// rawCommentUser / rawComment / rawBeReplied 是上游评论 JSON 的解码形状。
// v2 列表与热门两接口的评论对象字段同构(user/commentId/content/time/likedCount/
// liked/beReplied),差异仅在外层信封(v2: data.comments;热门: hotComments 顶层),
// 解码器共用。
type rawCommentUser struct {
	UserID    int64  `json:"userId"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
	VipType   int32  `json:"vipType"`
}

type rawBeReplied struct {
	BeRepliedCommentID int64           `json:"beRepliedCommentId"`
	Content            string          `json:"content"`
	User               *rawCommentUser `json:"user"`
}

type rawComment struct {
	CommentID  int64           `json:"commentId"`
	Content    string          `json:"content"`
	Time       int64           `json:"time"`
	TimeStr    string          `json:"timeStr"`
	LikedCount int64           `json:"likedCount"`
	Liked      bool            `json:"liked"`
	ReplyCount int32           `json:"replyCount"`
	User       *rawCommentUser `json:"user"`
	BeReplied  []rawBeReplied  `json:"beReplied"`
	ShowFloor  *rawShowFloor   `json:"showFloorComment"`
}

// rawShowFloor 是楼中楼摘要;热门评论接口无顶层 replyCount,用它兜底。
type rawShowFloor struct {
	ReplyCount int32 `json:"replyCount"`
}

// toComment 把上游评论对象转 proto。beReplied 上游为 null 或单元素数组
// (楼层引用),null 解码为零值切片,不报错。
func toComment(c rawComment) *mmpb.Comment {
	replyCount := c.ReplyCount
	if replyCount == 0 && c.ShowFloor != nil {
		replyCount = c.ShowFloor.ReplyCount
	}
	user := &mmpb.CommentUser{}
	if c.User != nil {
		user = &mmpb.CommentUser{
			UserId: c.User.UserID, Nickname: c.User.Nickname,
			AvatarUrl: c.User.AvatarURL, VipType: c.User.VipType,
		}
	}
	out := &mmpb.Comment{
		CommentId: c.CommentID, Content: c.Content, Time: c.Time, TimeStr: c.TimeStr,
		LikedCount: c.LikedCount, Liked: c.Liked, ReplyCount: replyCount, User: user,
	}
	for _, br := range c.BeReplied {
		item := &mmpb.BeRepliedComment{CommentId: br.BeRepliedCommentID, Content: br.Content}
		if br.User != nil {
			item.User = &mmpb.CommentUser{
				UserId: br.User.UserID, Nickname: br.User.Nickname,
				AvatarUrl: br.User.AvatarURL, VipType: br.User.VipType,
			}
		}
		out.BeReplied = append(out.BeReplied, item)
	}
	return out
}

// List 是评论列表的接口声明(上游 v2,eapi)。
//
// 上游 /api/v2/resource/comments(HTTP 发 /eapi/...),body 带 threadId 与分页。
// 分页语义按上游 sortType:推荐/热度以页码换算 cursor,时间排序只认 cursor
// (换算规则与 NeteaseCloudMusicApi comment/new 一致,2026-10 真实上游验证可用)。
var List = &engine.Endpoint[*mmpb.CommentListRequest, *mmpb.CommentListResponse]{
	Meta: readMeta("/eapi/v2/resource/comments", engine.CryptoEAPI),
	NewResp: func() *mmpb.CommentListResponse {
		return &mmpb.CommentListResponse{}
	},
	MapRequest: func(req *mmpb.CommentListRequest) (map[string]any, error) {
		tid, err := threadID(req.GetResourceType(), req.GetResourceId())
		if err != nil {
			return nil, err
		}
		pageNo := int(req.GetPageNo())
		if pageNo <= 0 {
			pageNo = 1
		}
		pageSize := int(req.GetPageSize())
		if pageSize <= 0 {
			pageSize = 20
		}
		var cursor any
		switch sortTypeProtoToUpstream(req.GetSort()) {
		case 2: // 热度
			cursor = fmt.Sprintf("normalHot#%d", (pageNo-1)*pageSize)
		case 3: // 时间:翻页只认 cursor,默认 "0"
			cursor = req.GetCursor()
			if cursor == "" {
				cursor = "0"
			}
		default: // 推荐
			cursor = (pageNo - 1) * pageSize
		}
		return map[string]any{
			"threadId":  tid,
			"pageNo":    pageNo,
			"pageSize":  pageSize,
			"cursor":    cursor,
			"sortType":  sortTypeProtoToUpstream(req.GetSort()),
			"showInner": true,
		}, nil
	},
	MapResponse: func(_ *mmpb.CommentListRequest, raw json.RawMessage) (*mmpb.CommentListResponse, error) {
		var resp struct {
			Data struct {
				Comments   []rawComment `json:"comments"`
				TotalCount int64        `json:"totalCount"`
				HasMore    bool         `json:"hasMore"`
				Cursor     string       `json:"cursor"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return &mmpb.CommentListResponse{}, fmt.Errorf("解析评论列表失败: %w", err)
		}
		out := &mmpb.CommentListResponse{
			TotalCount: resp.Data.TotalCount,
			HasMore:    resp.Data.HasMore,
			Cursor:     resp.Data.Cursor,
		}
		for _, c := range resp.Data.Comments {
			out.Comments = append(out.Comments, toComment(c))
		}
		return out, nil
	},
}

// Hot 是热门评论的接口声明(weapi,threadId 拼进 path,故声明 PathFunc)。
//
// 上游 /weapi/v1/resource/hotcomments/{threadId},body 带 rid / 分页。
// 响应顶层即 hotComments / topComments / total / hasMore(无 data 信封)。
var Hot = &engine.Endpoint[*mmpb.HotCommentsRequest, *mmpb.HotCommentsResponse]{
	Meta: readMeta("/weapi/v1/resource/hotcomments", engine.CryptoWeAPI), // 占位,实际 path 由 PathFunc 生成
	PathFunc: func(req *mmpb.HotCommentsRequest) string {
		tid, err := threadID(req.GetResourceType(), req.GetResourceId())
		if err != nil {
			return "" // MapRequest 已先行校验并报错,这里不可达
		}
		return "/weapi/v1/resource/hotcomments/" + tid
	},
	NewResp: func() *mmpb.HotCommentsResponse {
		return &mmpb.HotCommentsResponse{}
	},
	MapRequest: func(req *mmpb.HotCommentsRequest) (map[string]any, error) {
		if _, err := threadID(req.GetResourceType(), req.GetResourceId()); err != nil {
			return nil, err
		}
		return map[string]any{
			"rid":        req.GetResourceId(),
			"limit":      req.GetLimit(),
			"offset":     req.GetOffset(),
			"beforeTime": req.GetBeforeTime(),
		}, nil
	},
	MapResponse: func(_ *mmpb.HotCommentsRequest, raw json.RawMessage) (*mmpb.HotCommentsResponse, error) {
		var resp struct {
			HotComments []rawComment `json:"hotComments"`
			TopComments []rawComment `json:"topComments"`
			Total       int64        `json:"total"`
			HasMore     bool         `json:"hasMore"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return &mmpb.HotCommentsResponse{}, fmt.Errorf("解析热门评论失败: %w", err)
		}
		out := &mmpb.HotCommentsResponse{Total: resp.Total, HasMore: resp.HasMore}
		for _, c := range resp.HotComments {
			out.HotComments = append(out.HotComments, toComment(c))
		}
		for _, c := range resp.TopComments {
			out.TopComments = append(out.TopComments, toComment(c))
		}
		return out, nil
	},
}
