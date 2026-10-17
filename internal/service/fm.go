// Package service 的 FMService impl。
package service

import (
	"context"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	fmendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/fm"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// FMServer 实现 FMServiceServer。
type FMServer struct {
	mmpb.UnimplementedFMServiceServer
	eng *engine.Engine
}

// NewFMServer 创建 FMServer。
func NewFMServer(eng *engine.Engine) *FMServer {
	return &FMServer{eng: eng}
}

// GetPersonalFM 获取私人 FM。恒一行。
func (s *FMServer) GetPersonalFM(ctx context.Context, req *mmpb.GetPersonalFMRequest) (*mmpb.GetPersonalFMResponse, error) {
	resp, err := engine.Execute(s.eng, ctx, fmendpoint.GetPersonalFM, req)
	return resp, err
}
