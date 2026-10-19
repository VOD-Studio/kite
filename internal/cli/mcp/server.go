// Package mcp 实现 kite mcp:本地 stdio MCP 服务,把只读的网易云接口暴露给 Claude / Cursor 等 AI 客户端。
//
// 与 CLI 一样直连 engine + endpoint 声明(engine.ExecuteWithCookie),不经 gRPC / service;
// 登录态用本机的 kite login(NETEASE_COOKIE 环境变量优先),每次调用时读取,重新登录后无需重启。
//
// 只读是硬约束:不暴露红心、收藏、歌单编辑等写操作,AI 不能替用户修改账号(server_test.go 守护)。
// stdout 是协议通道,本包不得向 stdout 打印任何东西。
package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	albumendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/album"
	artistendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/artist"
	playlistendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/playlist"
	recommendendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/recommend"
	searchendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/search"
	songendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/song"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// Deps 是 MCP 服务的依赖:执行 endpoint 的 engine,以及每次调用时取当前 cookie 的函数(未登录返回空串)。
type Deps struct {
	Engine *engine.Engine
	Cookie func() string
}

// NewServer 构造注册了全部只读工具的 MCP 服务。version 用于向客户端报告实现版本。
func NewServer(d Deps, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "kite", Title: "kite 网易云音乐", Version: version}, nil)
	for _, register := range tools {
		register.add(s, d)
	}
	return s
}

// Run 以 stdio 传输运行 MCP 服务,直到客户端断开或 ctx 取消。
func Run(ctx context.Context, d Deps, version string) error {
	return NewServer(d, version).Run(ctx, &mcp.StdioTransport{})
}

// tool 是一个已绑定 endpoint 的工具声明;RPC 是它消费的 proto rpc(Service/Method),供命令树守护登记。
type tool struct {
	Name string
	RPC  string
	Meta engine.Meta
	add  func(s *mcp.Server, d Deps)
}

// readTool 声明一个只读工具:参数 In 经 build 转成 proto 请求,用 endpoint 执行,结果以 protojson 文本返回。
func readTool[In any, Req proto.Message, Resp proto.Message](
	name, rpc, desc string, ep *engine.Endpoint[Req, Resp], build func(In) Req,
) tool {
	return tool{
		Name: name,
		RPC:  rpc,
		Meta: ep.Meta,
		add: func(s *mcp.Server, d Deps) {
			mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
				func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
					resp, err := engine.ExecuteWithCookie(d.Engine, engine.WithCookie(ctx, d.Cookie()), ep, build(in))
					if err != nil {
						return nil, nil, err
					}
					text, err := protojson.Marshal(resp)
					if err != nil {
						return nil, nil, err
					}
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(text)}}}, nil, nil
				})
		},
	}
}

// 工具参数。没有 omitempty 的字段是必填。
type (
	searchIn struct {
		Keyword string `json:"keyword" jsonschema:"搜索关键词,如歌名、歌手名"`
		Limit   int32  `json:"limit,omitempty" jsonschema:"返回条数,默认 30"`
		Offset  int32  `json:"offset,omitempty" jsonschema:"分页偏移,默认 0"`
	}
	songIn struct {
		SongID int64 `json:"song_id" jsonschema:"网易云歌曲 ID"`
	}
	songURLIn struct {
		SongID int64  `json:"song_id" jsonschema:"网易云歌曲 ID"`
		Level  string `json:"level,omitempty" jsonschema:"音质:standard(默认)/ exhigh / lossless / hires"`
	}
	playlistIn struct {
		PlaylistID int64 `json:"playlist_id" jsonschema:"网易云歌单 ID"`
	}
	albumIn struct {
		AlbumID int64 `json:"album_id" jsonschema:"网易云专辑 ID"`
	}
	artistIn struct {
		ArtistID int64 `json:"artist_id" jsonschema:"网易云歌手 ID"`
	}
	similarIn struct {
		SongID int64 `json:"song_id" jsonschema:"网易云歌曲 ID"`
		Limit  int32 `json:"limit,omitempty" jsonschema:"返回条数"`
	}
	noIn struct{}
)

var songLevels = map[string]mmpb.SongLevel{
	"standard": mmpb.SongLevel_SONG_LEVEL_STANDARD,
	"exhigh":   mmpb.SongLevel_SONG_LEVEL_EXHIGH,
	"lossless": mmpb.SongLevel_SONG_LEVEL_LOSSLESS,
	"hires":    mmpb.SongLevel_SONG_LEVEL_HRES,
}

// tools 是全部工具。只读:新增工具只能用查询类 endpoint(server_test.go 校验)。
var tools = []tool{
	readTool("search_songs", "SearchService/Search",
		"按关键词搜索网易云单曲,返回歌曲 ID、名称、歌手、专辑。拿到歌曲 ID 后可调用其它工具。",
		searchendpoint.Search, func(in searchIn) *mmpb.SearchRequest {
			return &mmpb.SearchRequest{Keyword: in.Keyword, Type: mmpb.SearchType_SEARCH_TYPE_SONG, Limit: in.Limit, Offset: in.Offset}
		}),
	readTool("get_song", "SongService/GetSongDetail", "获取歌曲详情:名称、歌手、专辑、时长、付费类型。",
		songendpoint.Detail, func(in songIn) *mmpb.GetSongDetailRequest { return &mmpb.GetSongDetailRequest{SongId: in.SongID} }),
	readTool("get_lyric", "SongService/GetLyric", "获取歌词:原始 LRC,有翻译 / 音译歌词时一并返回。",
		songendpoint.Lyric, func(in songIn) *mmpb.GetLyricRequest { return &mmpb.GetLyricRequest{SongId: in.SongID} }),
	readTool("get_song_url", "SongService/GetSongURL",
		"获取歌曲播放直链(几十分钟后过期)。已 kite login 时按账号权益取(VIP 账号可得 VIP 歌);未登录只有免费歌,VIP / 无版权的歌 url 为空。",
		songendpoint.URL, func(in songURLIn) *mmpb.GetSongURLRequest {
			return &mmpb.GetSongURLRequest{SongId: in.SongID, Level: songLevels[in.Level]}
		}),
	readTool("get_playlist", "PlaylistService/GetPlaylist", "获取歌单详情与曲目。",
		playlistendpoint.GetPlaylist, func(in playlistIn) *mmpb.GetPlaylistRequest {
			return &mmpb.GetPlaylistRequest{PlaylistId: in.PlaylistID}
		}),
	readTool("get_album", "AlbumService/GetAlbum", "获取专辑详情与曲目。",
		albumendpoint.GetAlbum, func(in albumIn) *mmpb.GetAlbumRequest { return &mmpb.GetAlbumRequest{AlbumId: in.AlbumID} }),
	readTool("get_artist", "ArtistService/GetArtist", "获取歌手详情。",
		artistendpoint.GetArtist, func(in artistIn) *mmpb.GetArtistRequest { return &mmpb.GetArtistRequest{ArtistId: in.ArtistID} }),
	readTool("get_artist_top_songs", "ArtistService/TopSongs", "获取歌手的热门歌曲。",
		artistendpoint.TopSongs, func(in artistIn) *mmpb.TopSongsRequest { return &mmpb.TopSongsRequest{ArtistId: in.ArtistID} }),
	readTool("similar_songs", "SongService/SimilarSongs", "获取与某首歌相似的歌曲。",
		songendpoint.SimilarSongs, func(in similarIn) *mmpb.SimilarSongsRequest {
			return &mmpb.SimilarSongsRequest{SongId: in.SongID, Limit: in.Limit}
		}),
	readTool("hot_searches", "SearchService/Hot", "获取当前热搜关键词。",
		searchendpoint.Hot, func(noIn) *mmpb.HotRequest { return &mmpb.HotRequest{} }),
	readTool("daily_recommend", "RecommendService/GetDailyRecommend",
		"获取每日推荐歌曲。需要先在本机执行 kite login,未登录时返回 LOGIN_REQUIRED。",
		recommendendpoint.GetDailyRecommend, func(noIn) *mmpb.GetDailyRecommendRequest { return &mmpb.GetDailyRecommendRequest{} }),
}

// RPCs 返回全部工具消费的 rpc,供 kite mcp 命令打命令树守护注解。
func RPCs() []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.RPC)
	}
	return out
}
