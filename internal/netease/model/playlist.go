// Package model 的歌单实体映射。
package model

import (
	"encoding/json"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// rawPlaylistDetail 是网易云歌单详情接口的响应。
type rawPlaylistDetail struct {
	Code     int `json:"code"` // 业务码
	Playlist struct {
		ID          int64     `json:"id"`          // 歌单ID
		Name        string    `json:"name"`        // 歌单名
		CoverImgURL string    `json:"coverImgUrl"` // 封面URL
		TrackCount  int       `json:"trackCount"`  // 歌曲总数
		Creator     rawUser   `json:"creator"`     // 创建者
		Tracks      []rawSong `json:"tracks"`      // 歌曲列表（详情返回前若干首）
		TrackIDs    []struct {
			ID int64 `json:"id"` // 歌曲ID
		} `json:"trackIds"` // 全部歌曲ID（大歌单分页拉取用）
	} `json:"playlist"` // 歌单信息
}

// PlaylistTrackIDs 是歌单的完整曲目 ID 列表(歌单原顺序)。
type PlaylistTrackIDs struct {
	IDs []int64
}

// DecodePlaylistTrackIDs 取歌单详情里完整的 trackIds。
//
// 详情内嵌的 tracks 会被上游截断(实测匿名请求时 71 首的歌单只返回 10 首、3672 首的只返回 6 首),
// 只有 trackIds 是完整的;按页取歌曲要用它切片,而不是对 tracks 切片。
func DecodePlaylistTrackIDs(raw json.RawMessage) (*PlaylistTrackIDs, error) {
	var r rawPlaylistDetail
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	out := &PlaylistTrackIDs{IDs: make([]int64, 0, len(r.Playlist.TrackIDs))}
	for _, t := range r.Playlist.TrackIDs {
		out.IDs = append(out.IDs, t.ID)
	}
	return out, nil
}

// MapPlaylist 把网易云原始歌单结构转成 proto Playlist。
func MapPlaylist(raw json.RawMessage) (*mmpb.Playlist, error) {
	var r rawPlaylistDetail
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	p := r.Playlist
	return &mmpb.Playlist{
		Id:         p.ID,
		Name:       p.Name,
		CoverUrl:   p.CoverImgURL,
		TrackCount: int32(p.TrackCount),
		Creator:    MapUser(p.Creator),
		Songs:      MapSongs(p.Tracks),
	}, nil
}
