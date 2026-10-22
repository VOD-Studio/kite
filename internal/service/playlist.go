// Package service 的 PlaylistServer:结构体与组合方法;一行转发方法由 playlist_gen.go 生成。
package service

import (
	"context"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/compose"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// PlaylistServer 实现 PlaylistServiceServer，持有 *engine.Engine。
type PlaylistServer struct {
	mmpb.UnimplementedPlaylistServiceServer
	eng *engine.Engine
}

// NewPlaylistServer 创建 PlaylistServer。
func NewPlaylistServer(eng *engine.Engine) *PlaylistServer {
	return &PlaylistServer{eng: eng}
}

// AllTracks 按歌单顺序分页获取歌曲(两次上游调用的组合,见 compose.PlaylistTracks)。
func (s *PlaylistServer) AllTracks(ctx context.Context, req *mmpb.AllTracksRequest) (*mmpb.AllTracksResponse, error) {
	return compose.PlaylistTracks(ctx, s.eng, req)
}
