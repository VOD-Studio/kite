// Package service 的 RecommendService impl 结构体;一行转发方法由 recommend_gen.go 生成。
package service

import (
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// RecommendServer 实现 RecommendServiceServer，持有 *engine.Engine。
type RecommendServer struct {
	mmpb.UnimplementedRecommendServiceServer
	eng *engine.Engine
}

// NewRecommendServer 创建 RecommendServer。
func NewRecommendServer(eng *engine.Engine) *RecommendServer {
	return &RecommendServer{eng: eng}
}
