// Package service 的 SearchService impl 结构体;一行转发方法由 search_gen.go 生成。
package service

import (
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// SearchServer 实现 SearchServiceServer，持有 *engine.Engine。
type SearchServer struct {
	mmpb.UnimplementedSearchServiceServer
	eng *engine.Engine
}

// NewSearchServer 创建 SearchServer。
func NewSearchServer(eng *engine.Engine) *SearchServer {
	return &SearchServer{eng: eng}
}
