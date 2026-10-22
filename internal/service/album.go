// Package service 的 AlbumService impl 结构体;一行转发方法由 album_gen.go 生成。
package service

import (
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// AlbumServer 实现 AlbumServiceServer，持有 *engine.Engine。
type AlbumServer struct {
	mmpb.UnimplementedAlbumServiceServer
	eng *engine.Engine
}

// NewAlbumServer 创建 AlbumServer。
func NewAlbumServer(eng *engine.Engine) *AlbumServer {
	return &AlbumServer{eng: eng}
}
