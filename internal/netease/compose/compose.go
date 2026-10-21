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
	"fmt"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	playlistendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/playlist"
	songendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/song"
	userendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/user"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// MaxPageSize 是单页上限,超过时按上限返回。批量歌曲详情一次最多 1000 首,
// 用户歌单的上游单页也按这个数取。
const MaxPageSize = songendpoint.MaxDetails

const (
	// defaultTracksLimit 是 AllTracks 未指定 limit 时的每页数量。
	defaultTracksLimit = 100
	// defaultPlaylistsLimit 是 UserPlaylist 未指定 limit 时的每页数量。
	defaultPlaylistsLimit = 30
	// maxPlaylistPages 防止上游一直返回 more=true 时无限翻页(最多取 MaxPageSize × 该值张歌单)。
	maxPlaylistPages = 20
)

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

// UserPlaylists 返回用户歌单的一页(对外的 UserPlaylist)。
//
// 上游分页不能直接透传:第一页会在 limit 之外额外插入「喜欢的音乐」,不返回总数,
// 创建 / 收藏的筛选也只能在拿到数据后做(先分页再筛选会得到短页、漏项)。所以按上游
// 单页上限取全量,再筛选、分页;total 是筛选后的总数,不随 limit / offset 变化。
// 「喜欢的音乐」的创建者就是用户本人,计入 ALL 与 CREATED。
func UserPlaylists(ctx context.Context, eng *engine.Engine, req *mmpb.UserPlaylistRequest) (*mmpb.UserPlaylistResponse, error) {
	all, err := allUserPlaylists(ctx, eng, req.GetUserId())
	if err != nil {
		return nil, err
	}
	matched := filterPlaylists(all, req.GetUserId(), req.GetFilter())
	lo, hi := window(len(matched), req.GetOffset(), req.GetLimit(), defaultPlaylistsLimit)
	return &mmpb.UserPlaylistResponse{Playlists: matched[lo:hi], Total: int32(len(matched))}, nil
}

// allUserPlaylists 按上游顺序取用户的全部歌单,按歌单 ID 去重。
//
// 新的一页没有任何新歌单时按已取完处理(防止上游一直返回同一页时死循环);
// 取满 maxPlaylistPages 页上游仍说还有时返回错误,不把不完整的列表当成完整结果。
func allUserPlaylists(ctx context.Context, eng *engine.Engine, userID int64) ([]*mmpb.Playlist, error) {
	var all []*mmpb.Playlist
	seen := map[int64]bool{}
	for page := range maxPlaylistPages {
		// 上游的 offset 按不含「喜欢的音乐」的列表计数,按请求的步长递增即可。
		resp, err := engine.Execute(eng, ctx, userendpoint.PlaylistPage, &mmpb.UserPlaylistRequest{
			UserId: userID, Limit: MaxPageSize, Offset: int32(page * MaxPageSize),
		})
		if err != nil {
			return nil, err
		}
		added := 0
		for _, p := range resp.Playlists {
			if seen[p.GetId()] {
				continue
			}
			seen[p.GetId()] = true
			all = append(all, p)
			added++
		}
		if !resp.More || added == 0 {
			return all, nil
		}
	}
	return nil, fmt.Errorf("%w: 取了 %d 页(%d 张)上游仍返回还有更多,无法给出完整的用户歌单", merrors.ErrInvalidResponse, maxPlaylistPages, len(all))
}

// filterPlaylists 按创建者区分创建 / 收藏。ownerUserID 为 0 时无法判断,原样返回。
func filterPlaylists(all []*mmpb.Playlist, ownerUserID int64, filter mmpb.PlaylistFilter) []*mmpb.Playlist {
	if ownerUserID == 0 || filter == mmpb.PlaylistFilter_PLAYLIST_FILTER_ALL || filter == mmpb.PlaylistFilter_PLAYLIST_FILTER_UNSPECIFIED {
		return all
	}
	wantCreated := filter == mmpb.PlaylistFilter_PLAYLIST_FILTER_CREATED
	out := make([]*mmpb.Playlist, 0, len(all))
	for _, p := range all {
		if (p.GetCreator().GetId() == ownerUserID) == wantCreated {
			out = append(out, p)
		}
	}
	return out
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
