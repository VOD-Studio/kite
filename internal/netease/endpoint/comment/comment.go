// Package comment 的评论域接口声明(#28 Phase A:endpoint + CLI 直连,
// proto 只落消息、不建 service,rpc 建模留第二阶段)。
//
// 读:List(评论列表,v2 eapi)与 Hot(热门评论,weapi,threadId 拼进 path)。
// 写:Like / Add / Reply / Delete(均需登录态,CLI 走 --yes 确认规范)。
package comment

import (
	"fmt"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// threadIDPrefix 资源类型 → 上游 threadId 前缀(网易云用前缀区分评论区,
// 与 NeteaseCloudMusicApi 的 resourceTypeMap 一致,2026-10 真实上游验证可用)。
var threadIDPrefix = map[mmpb.CommentResourceType]string{
	mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_SONG:     "R_SO_4_",
	mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_MV:       "R_MV_5_",
	mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_PLAYLIST: "A_PL_0_",
	mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_ALBUM:    "R_AL_3_",
}

// threadID 构造上游 threadId(前缀 + 资源 ID)。资源类型未指定时报错,
// 各 endpoint 的 MapRequest / PathFunc 均先经此校验,不让拼坏的 ID 打到上游。
func threadID(t mmpb.CommentResourceType, id int64) (string, error) {
	prefix, ok := threadIDPrefix[t]
	if !ok {
		return "", fmt.Errorf("未知的评论资源类型: %d", t)
	}
	return fmt.Sprintf("%s%d", prefix, id), nil
}

// sortTypeProtoToUpstream proto 排序枚举 → 上游 sortType 数值(99 推荐 / 2 热度 / 3 时间)。
// 未指定按推荐处理(与上游默认一致)。
func sortTypeProtoToUpstream(s mmpb.CommentSortType) int {
	switch s {
	case mmpb.CommentSortType_COMMENT_SORT_TYPE_HOT:
		return 2
	case mmpb.CommentSortType_COMMENT_SORT_TYPE_TIME:
		return 3
	default: // UNSPECIFIED / RECOMMEND
		return 99
	}
}
