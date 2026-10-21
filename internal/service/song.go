// Package service 是 grpc service impl 层（薄路由，无业务逻辑）。
//
// 一行转发方法由 *_gen.go 生成(kite-gen-service,见 internal/servicegen);
// 本目录手写的只有各 Server 结构体 / 构造器和带副作用或组合逻辑的方法。
// service 只是 gRPC server interface 的 adapter，只被 internal/server 引用；
// CLI 与 TUI 不经过它，共享边界是 engine + endpoint（见 docs/adr/shared-boundary-engine.md）。
package service

import (
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// SongServer 实现 SongServiceServer，持有 *engine.Engine。
type SongServer struct {
	mmpb.UnimplementedSongServiceServer
	eng *engine.Engine
}

// NewSongServer 创建 SongServer。
func NewSongServer(eng *engine.Engine) *SongServer {
	return &SongServer{eng: eng}
}
