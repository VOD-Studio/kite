// Package service 的 UserServer:结构体与组合方法;一行转发方法由 user_gen.go 生成。
package service

import (
	"context"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/compose"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// UserServer 实现 UserServiceServer，持有 *engine.Engine。
type UserServer struct {
	mmpb.UnimplementedUserServiceServer
	eng *engine.Engine
}

// NewUserServer 创建 UserServer。
func NewUserServer(eng *engine.Engine) *UserServer {
	return &UserServer{eng: eng}
}

// UserPlaylist 获取用户歌单列表(取全量后筛选、分页,见 compose.UserPlaylists)。
func (s *UserServer) UserPlaylist(ctx context.Context, req *mmpb.UserPlaylistRequest) (*mmpb.UserPlaylistResponse, error) {
	return compose.UserPlaylists(ctx, s.eng, req)
}
