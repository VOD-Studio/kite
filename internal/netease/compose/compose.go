// Package compose 承载需要多次调用上游才能完成的读取(跨接口组合,
// 见 docs/adr/shared-boundary-engine.md「引入真正的 use-case 层」的触发条件)。
//
// 每一步仍是一个 endpoint 声明、经 engine.Execute 执行;这里只负责编排步骤与分页。
// gRPC service、CLI 与其它前端都调这里,不得各自再拼一遍。
//
// 组合用到的 endpoint 都是 AuthOptional:调用方经 ctx 带了网易云 cookie(gRPC 的
// x-netease-cookie、CLI 的本地登录态)就用它,没带就匿名,任何情况下都不借用会话池。
// 因此 engine.Execute 与 CLI 惯用的 ExecuteWithCookie 在这里的凭据行为一致。
package compose

import (
	"context"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	playlistendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/playlist"
	songendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/song"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// MaxPageSize 是单页上限,超过时按上限返回(批量歌曲详情一次最多 1000 首)。
const MaxPageSize = songendpoint.MaxDetails

// defaultTracksLimit 是 AllTracks 未指定 limit 时的每页数量。
const defaultTracksLimit = 100

// PlaylistTracks 按歌单原顺序分页返回歌曲(对外的 AllTracks)。
//
// 歌单详情内嵌的 tracks 会被上游截断,只有 trackIds 完整:先取完整 trackIds,
// 切出这一页,再批量查这一页的歌曲详情。total 是歌单的歌曲总数;
// offset 超出总数时返回空页,不再请求歌曲详情。
func PlaylistTracks(ctx context.Context, eng *engine.Engine, req *mmpb.AllTracksRequest) (*mmpb.AllTracksResponse, error) {
	ids, err := engine.Execute(eng, ctx, playlistendpoint.TrackIDs, &mmpb.GetPlaylistRequest{PlaylistId: req.GetPlaylistId()})
	if err != nil {
		return nil, err
	}
	lo, hi := window(len(ids.IDs), req.GetOffset(), req.GetLimit(), defaultTracksLimit)
	out := &mmpb.AllTracksResponse{Total: int32(len(ids.IDs))}
	if lo == hi {
		return out, nil
	}
	songs, err := engine.Execute(eng, ctx, songendpoint.Details, ids.IDs[lo:hi])
	if err != nil {
		return nil, err
	}
	out.Songs = songs
	return out, nil
}

// window 把 offset / limit 换算成 [lo, hi) 下标:limit ≤ 0 用默认值,超过 MaxPageSize 按上限,
// offset 为负按 0,超出总数时得到空区间。
func window(total int, offset, limit int32, defaultLimit int) (lo, hi int) {
	n := int(limit)
	if n <= 0 {
		n = defaultLimit
	}
	n = min(n, MaxPageSize)
	lo = min(max(int(offset), 0), total)
	return lo, min(lo+n, total)
}
