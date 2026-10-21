// Package service 的 ArtistService impl 结构体;一行转发方法由 artist_gen.go 生成。
package service

import (
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// ArtistServer 实现 ArtistServiceServer，持有 *engine.Engine。
type ArtistServer struct {
	mmpb.UnimplementedArtistServiceServer
	eng *engine.Engine
}

// NewArtistServer 创建 ArtistServer。
func NewArtistServer(eng *engine.Engine) *ArtistServer {
	return &ArtistServer{eng: eng}
}
