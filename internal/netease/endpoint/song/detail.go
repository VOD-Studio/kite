// Package endpoint 定义网易云每个接口的声明。
//
// 每个文件一个接口的 Endpoint 声明（Meta + CachePolicy + MapRequest + MapResponse），
// 不是活跃服务。grpc service 层（internal/service）的每个方法恒一行调用 engine.Execute
// （需要多次上游调用的读取调 internal/netease/compose）。
package song

import (
	"encoding/json"
	"fmt"
	"time"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/model"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// Detail 是获取歌曲详情的接口声明。
// Req/Resp 都用 proto 生成的指针类型（gRPC 天然传指针）。
var Detail = &engine.Endpoint[*mmpb.GetSongDetailRequest, *mmpb.GetSongDetailResponse]{
	Meta: engine.Meta{
		Path:   "/weapi/v3/song/detail",
		Method: "POST",
		Crypto: engine.CryptoWeAPI,
		Auth:   session.AuthAnonymous,
	},
	Cache: &engine.CachePolicy[*mmpb.GetSongDetailRequest]{
		Key: func(req *mmpb.GetSongDetailRequest) string {
			return fmt.Sprintf("song:detail:%d", req.GetSongId())
		},
		TTL: 24 * time.Hour,
	},
	NewResp: func() *mmpb.GetSongDetailResponse { return &mmpb.GetSongDetailResponse{} },
	MapRequest: func(req *mmpb.GetSongDetailRequest) (map[string]any, error) {
		idStr := fmt.Sprintf("%d", req.GetSongId())
		return map[string]any{
			"c":   fmt.Sprintf(`[{"id":%s}]`, idStr),
			"ids": fmt.Sprintf("[%s]", idStr),
		}, nil
	},
	MapResponse: func(_ *mmpb.GetSongDetailRequest, raw json.RawMessage) (*mmpb.GetSongDetailResponse, error) {
		songs, err := model.DecodeSongDetail(raw)
		if err != nil {
			return &mmpb.GetSongDetailResponse{}, err
		}
		if len(songs) == 0 {
			return &mmpb.GetSongDetailResponse{}, fmt.Errorf("歌曲不存在: %w", merrors.ErrNotFound)
		}
		return &mmpb.GetSongDetailResponse{Song: songs[0]}, nil
	},
}

// MaxDetails 是批量歌曲详情一次最多查询的歌曲数(实测 1001 首时上游返回 400「参数错误」)。
const MaxDetails = 1000

// Details 批量取歌曲详情,按请求的 ID 顺序返回;上游查不到的歌曲跳过。
//
// 不直接对外,供 compose.PlaylistTracks 取一页歌曲。AuthOptional:带了 cookie 时
// 版权与 VIP 信息按调用方账号返回,没带就匿名,不借用会话池。
var Details = &engine.Endpoint[[]int64, []*mmpb.Song]{
	Meta: engine.Meta{
		Path:   "/weapi/v3/song/detail",
		Method: "POST",
		Crypto: engine.CryptoWeAPI,
		Auth:   session.AuthOptional,
	},
	MapRequest: func(ids []int64) (map[string]any, error) {
		if len(ids) == 0 || len(ids) > MaxDetails {
			return nil, fmt.Errorf("批量歌曲详情一次 1~%d 首,收到 %d 首", MaxDetails, len(ids))
		}
		c := make([]map[string]int64, len(ids))
		for i, id := range ids {
			c[i] = map[string]int64{"id": id}
		}
		data, err := json.Marshal(c)
		if err != nil {
			return nil, err
		}
		return map[string]any{"c": string(data)}, nil
	},
	MapResponse: func(ids []int64, raw json.RawMessage) ([]*mmpb.Song, error) {
		songs, err := model.DecodeSongDetail(raw)
		if err != nil {
			return nil, err
		}
		// 上游不保证按请求顺序返回。
		byID := make(map[int64]*mmpb.Song, len(songs))
		for _, s := range songs {
			byID[s.GetId()] = s
		}
		out := make([]*mmpb.Song, 0, len(ids))
		for _, id := range ids {
			if s, ok := byID[id]; ok {
				out = append(out, s)
			}
		}
		return out, nil
	},
}
