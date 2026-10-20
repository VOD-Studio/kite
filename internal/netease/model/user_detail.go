// Package model 的用户详情解码。
package model

import (
	"encoding/json"
	"fmt"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// rawUserDetail 是网易云用户详情接口的响应。
type rawUserDetail struct {
	Code    int `json:"code"`  // 业务码
	Level   int `json:"level"` // 用户等级
	Profile struct {
		UserID    int64  `json:"userId"`    // 用户ID
		Nickname  string `json:"nickname"`  // 昵称
		AvatarURL string `json:"avatarUrl"` // 头像URL
		Gender    int    `json:"gender"`    // 性别（0未知 1男 2女）
		Signature string `json:"signature"` // 个人签名
		Followeds int64  `json:"followeds"` // 粉丝数
		Follows   int64  `json:"follows"`   // 关注数
	} `json:"profile"` // 用户资料
}

// DecodeUserDetail 解析用户详情响应。
func DecodeUserDetail(raw json.RawMessage) (*mmpb.UserDetail, error) {
	var r rawUserDetail
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("解析用户详情失败: %w", err)
	}
	if r.Profile.UserID == 0 {
		return nil, fmt.Errorf("用户不存在: %w", merrors.ErrNotFound)
	}
	p := r.Profile
	return &mmpb.UserDetail{
		UserId:    p.UserID,
		Nickname:  p.Nickname,
		AvatarUrl: p.AvatarURL,
		Gender:    int32(p.Gender),
		Signature: p.Signature,
		Level:     int32(r.Level),
		Followeds: p.Followeds,
		Follows:   p.Follows,
	}, nil
}

// rawUserSubCount 是用户数量统计响应。
type rawUserSubCount struct {
	Code          int `json:"code"`           // 业务码
	PlaylistCount int `json:"playlistCount"`  // 创建的歌单数
	DjRadioCount  int `json:"djRadiosCount"`  // DJ电台数
	MvCount       int `json:"mvCount"`        // 收藏MV数
	ArtistCount   int `json:"artistCount"`    // 收藏歌手数
	NewAlbumCount int `json:"newAlbumsCount"` // 收藏专辑数
}

// DecodeUserSubCount 解析用户数量统计响应。
func DecodeUserSubCount(raw json.RawMessage) (*mmpb.UserSubCount, error) {
	var r rawUserSubCount
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("解析用户统计失败: %w", err)
	}
	return &mmpb.UserSubCount{
		PlaylistCount: int32(r.PlaylistCount),
		DjRadioCount:  int32(r.DjRadioCount),
		MvCount:       int32(r.MvCount),
		ArtistCount:   int32(r.ArtistCount),
		NewAlbumCount: int32(r.NewAlbumCount),
	}, nil
}

// rawUserPlaylists 是用户歌单列表响应。
type rawUserPlaylists struct {
	Code     int  `json:"code"` // 业务码
	More     bool `json:"more"` // 是否还有更多
	Playlist []struct {
		ID          int64  `json:"id"`          // 歌单ID
		Name        string `json:"name"`        // 歌单名
		CoverImgURL string `json:"coverImgUrl"` // 封面URL
		PlayCount   int64  `json:"playCount"`   // 播放数
		TrackCount  int    `json:"trackCount"`  // 曲目数
		Creator     struct {
			UserID   int64  `json:"userId"`   // 创建者用户ID（判断创建/收藏用）
			Nickname string `json:"nickname"` // 创建者昵称
		} `json:"creator"` // 创建者
	} `json:"playlist"` // 歌单列表
}

// UserPlaylistPage 是上游用户歌单接口的一页原始结果(未筛选)。
type UserPlaylistPage struct {
	// Playlists 按上游顺序。第一页会在 limit 之外额外带上「喜欢的音乐」。
	Playlists []*mmpb.Playlist
	// More 表示上游还有下一页。上游不返回总数。
	More bool
}

// DecodeUserPlaylistPage 解析用户歌单接口的一页。筛选与分页在 compose.UserPlaylists 里做:
// 上游的分页既不返回总数、第一页又多一条,只有拿到全量后才能给出准确的 total。
func DecodeUserPlaylistPage(raw json.RawMessage) (*UserPlaylistPage, error) {
	var r rawUserPlaylists
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("解析用户歌单失败: %w", err)
	}
	out := &UserPlaylistPage{More: r.More, Playlists: make([]*mmpb.Playlist, 0, len(r.Playlist))}
	for _, p := range r.Playlist {
		out.Playlists = append(out.Playlists, &mmpb.Playlist{
			Id:         p.ID,
			Name:       p.Name,
			CoverUrl:   p.CoverImgURL,
			PlayCount:  p.PlayCount,
			TrackCount: int32(p.TrackCount),
			Creator: &mmpb.User{
				Id:       p.Creator.UserID,
				Nickname: p.Creator.Nickname,
			},
		})
	}
	return out, nil
}
