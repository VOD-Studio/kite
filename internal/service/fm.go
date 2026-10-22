// Package service 的 FMService impl 结构体;一行转发方法由 fm_gen.go 生成。
package service

import (
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// FMServer 实现 FMServiceServer，持有 *engine.Engine。
type FMServer struct {
	mmpb.UnimplementedFMServiceServer
	eng *engine.Engine
}

// NewFMServer 创建 FMServer。
func NewFMServer(eng *engine.Engine) *FMServer {
	return &FMServer{eng: eng}
}
