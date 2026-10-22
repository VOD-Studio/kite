// Package model 的官方榜单解码。
package model

import (
	"encoding/json"
	"fmt"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// rawToplist 是榜单列表项(/toplist 与 /toplist/detail 的 list 元素同构)。
type rawToplist struct {
	ID              int64                   `json:"id"`              // 榜单ID(即歌单ID)
	Name            string                  `json:"name"`            // 榜单名
	CoverImgURL     string                  `json:"coverImgUrl"`     // 封面URL
	UpdateFrequency string                  `json:"updateFrequency"` // 更新周期(如「每天更新」)
	Description     string                  `json:"description"`     // 榜单描述
	PlayCount       float64                 `json:"playCount"`       // 播放数(大数字可能科学计数法,float64 容纳)
	TrackCount      int                     `json:"trackCount"`      // 歌曲总数
	Tracks          []rawToplistPreviewSong `json:"tracks"`          // 前三歌曲概要(仅 /toplist/detail 的主榜有,其余为 null)
}

// rawToplistPreviewSong 是榜单详情里的前三歌曲概要。上游只给两个字符串:
// first=歌名,second=歌手名串;second 里的「/」是分隔符与艺名内容的混合
// (如「K/DA/英雄联盟」),不可拆分。
type rawToplistPreviewSong struct {
	First  string `json:"first"`  // 歌名
	Second string `json:"second"` // 歌手名串
}

// rawToplistsResponse 是 /toplist 与 /toplist/detail 的响应(list 元素同构,
// 区别只在主榜是否带 tracks 概要;artistToplist/rewardToplist 是独立的歌手榜 /
// 赞赏榜结构,歌手榜由 artist 域的 TopArtists 提供,此处不解码)。
type rawToplistsResponse struct {
	Code int          `json:"code"` // 业务码
	List []rawToplist `json:"list"` // 榜单列表
}

// DecodeToplists 解析榜单列表响应(/toplist 与 /toplist/detail 共用)。
func DecodeToplists(raw json.RawMessage) ([]*mmpb.Toplist, error) {
	var r rawToplistsResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("解析榜单列表失败: %w", err)
	}
	out := make([]*mmpb.Toplist, 0, len(r.List))
	for _, tl := range r.List {
		previews := make([]*mmpb.ToplistPreviewTrack, 0, len(tl.Tracks))
		for _, t := range tl.Tracks {
			previews = append(previews, &mmpb.ToplistPreviewTrack{Name: t.First, Artists: t.Second})
		}
		out = append(out, &mmpb.Toplist{
			Id: tl.ID, Name: tl.Name, CoverUrl: tl.CoverImgURL,
			UpdateFrequency: tl.UpdateFrequency, Description: tl.Description,
			PlayCount: int64(tl.PlayCount), TrackCount: int32(tl.TrackCount),
			PreviewTracks: previews,
		})
	}
	return out, nil
}
