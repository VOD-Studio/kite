// Package comment 的写操作接口声明(需登录态,cookie override 路径)。
//
// 与收藏专辑同一形态:Cache=nil(不缓存),Auth=AuthLoggedIn;响应仅 code
// (engine 统一校验),MapResponse 回填操作后状态或尽量解出评论 ID。
// 匿名调用会被上游拦截(code=301,engine 映射「登录态失效」)——2026-10 真实
// 上游验证,登录态下的实际行为本机无 cookie,未验证(见 #28 交付说明)。
package comment

import (
	"encoding/json"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// writeMeta 构造评论写接口的 Meta(weapi POST + 需登录态)。
func writeMeta(path string) engine.Meta {
	return engine.Meta{Path: path, Method: "POST", Crypto: engine.CryptoWeAPI, Auth: session.AuthLoggedIn}
}

// Like 是点赞/取消点赞评论的接口声明(同一路径族,like/unlike 由 PathFunc 派生)。
//
// 上游 /weapi/v1/comment/like 与 /weapi/v1/comment/unlike,body 相同(threadId、
// commentId),只有 path 不同,故单 endpoint + PathFunc 按 like 取 path
// (语义与 song.Like 的 like 布尔一致,Phase 2 建模为一个 rpc)。
var Like = &engine.Endpoint[*mmpb.LikeCommentRequest, *mmpb.LikeCommentResponse]{
	Meta: writeMeta("/weapi/v1/comment/like"), // 占位,实际 path 由 PathFunc 生成
	PathFunc: func(req *mmpb.LikeCommentRequest) string {
		if req.GetLike() {
			return "/weapi/v1/comment/like"
		}
		return "/weapi/v1/comment/unlike"
	},
	NewResp: func() *mmpb.LikeCommentResponse { return &mmpb.LikeCommentResponse{} },
	MapRequest: func(req *mmpb.LikeCommentRequest) (map[string]any, error) {
		tid, err := threadID(req.GetResourceType(), req.GetResourceId())
		if err != nil {
			return nil, err
		}
		return map[string]any{"threadId": tid, "commentId": req.GetCommentId()}, nil
	},
	MapResponse: func(req *mmpb.LikeCommentRequest, _ json.RawMessage) (*mmpb.LikeCommentResponse, error) {
		// 响应仅 code(engine 校验),回填请求里的 like 作为操作后状态。
		return &mmpb.LikeCommentResponse{Liked: req.GetLike()}, nil
	},
}

// Add 是发表评论的接口声明(上游 /weapi/resource/comments/add)。
var Add = &engine.Endpoint[*mmpb.AddCommentRequest, *mmpb.AddCommentResponse]{
	Meta:    writeMeta("/weapi/resource/comments/add"),
	NewResp: func() *mmpb.AddCommentResponse { return &mmpb.AddCommentResponse{} },
	MapRequest: func(req *mmpb.AddCommentRequest) (map[string]any, error) {
		tid, err := threadID(req.GetResourceType(), req.GetResourceId())
		if err != nil {
			return nil, err
		}
		return map[string]any{"threadId": tid, "content": req.GetContent()}, nil
	},
	MapResponse: func(_ *mmpb.AddCommentRequest, raw json.RawMessage) (*mmpb.AddCommentResponse, error) {
		// 上游成功响应可能带回新评论对象(comment.id);拿不到时不报错,
		// 成功与否以 code 校验(engine)为准。登录态形状未实测,见文件头说明。
		var resp struct {
			Comment struct {
				ID int64 `json:"id"`
			} `json:"comment"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return &mmpb.AddCommentResponse{}, nil //nolint:nilerr // 响应形状未核对过,解码失败不当作业务失败
		}
		return &mmpb.AddCommentResponse{CommentId: resp.Comment.ID}, nil
	},
}

// Reply 是回复评论的接口声明(上游 /weapi/resource/comments/reply)。
var Reply = &engine.Endpoint[*mmpb.ReplyCommentRequest, *mmpb.ReplyCommentResponse]{
	Meta:    writeMeta("/weapi/resource/comments/reply"),
	NewResp: func() *mmpb.ReplyCommentResponse { return &mmpb.ReplyCommentResponse{} },
	MapRequest: func(req *mmpb.ReplyCommentRequest) (map[string]any, error) {
		tid, err := threadID(req.GetResourceType(), req.GetResourceId())
		if err != nil {
			return nil, err
		}
		return map[string]any{"threadId": tid, "commentId": req.GetCommentId(), "content": req.GetContent()}, nil
	},
	MapResponse: func(_ *mmpb.ReplyCommentRequest, raw json.RawMessage) (*mmpb.ReplyCommentResponse, error) {
		var resp struct {
			Comment struct {
				ID int64 `json:"id"`
			} `json:"comment"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return &mmpb.ReplyCommentResponse{}, nil //nolint:nilerr // 同 Add:解码失败不当作业务失败
		}
		return &mmpb.ReplyCommentResponse{CommentId: resp.Comment.ID}, nil
	},
}

// Delete 是删除评论的接口声明(上游 /weapi/resource/comments/delete,只能删自己的)。
var Delete = &engine.Endpoint[*mmpb.DeleteCommentRequest, *mmpb.DeleteCommentResponse]{
	Meta:    writeMeta("/weapi/resource/comments/delete"),
	NewResp: func() *mmpb.DeleteCommentResponse { return &mmpb.DeleteCommentResponse{} },
	MapRequest: func(req *mmpb.DeleteCommentRequest) (map[string]any, error) {
		tid, err := threadID(req.GetResourceType(), req.GetResourceId())
		if err != nil {
			return nil, err
		}
		return map[string]any{"threadId": tid, "commentId": req.GetCommentId()}, nil
	},
	MapResponse: func(_ *mmpb.DeleteCommentRequest, _ json.RawMessage) (*mmpb.DeleteCommentResponse, error) {
		// 响应仅 code(engine 校验),无业务载荷。
		return &mmpb.DeleteCommentResponse{}, nil
	},
}
