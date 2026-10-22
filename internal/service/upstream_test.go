//go:build upstream

package service

// 真实上游冒烟:对网易云真实接口逐个调用服务端实际使用的 service 方法
// (评论域 #28 Phase A 尚无 service rpc,对应用例直连 endpoint 声明执行)。
//
// 为什么需要它:服务端 RPC 曾有 5 个对真实上游不可用(2 个 NOT_FOUND,3 个「成功但返回空」),
// 而所有单元测试都是绿的——夹具是手写的,与解码器出自同一个假设,上游改了路径 / 字段名 / 字段类型
// 都发现不了。本测试直接问真实上游。
//
// 不进默认 CI(外部依赖会让门禁「没改代码也变红」),手动运行:
//
//	make upstream-smoke
//
// 断言刻意宽松:只要求「调用成功且响应不是空消息」(覆盖「成功但返回空」这一整类问题),
// 个别接口加了具体字段断言。需要登录态的 RPC 不在此列(无 cookie 会返回 LOGIN_REQUIRED,是预期行为)。
// 样例 ID 取自长期稳定的公开资源。

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cache"
	commentendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/comment"
	mvendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/mv"
	toplistendpoint "github.com/VOD-Studio/kite/internal/netease/endpoint/toplist"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

const (
	songID     = 347230   // Beyond《海阔天空》
	albumID    = 34209    // 《乐与怒》
	artistID   = 6452     // 周杰伦
	playlistID = 3778678  // 热歌榜
	userID     = 32953014 // 网易云 API 作者的公开账号
	mvID       = 5436712  // 莫文蔚《广岛之恋》MV(MV 域 #32 冒烟)
	// commentMvID 是公开 MV(评论区样例;评论域 #28 冒烟引入)。
	// stablePlaylistID 是评论区稳定的普通歌单(榜单歌单的热评会随榜单更新清空,
	// 不能拿来做热评冒烟;ID 与 endpoint/playlist 的 testdata 夹具同源)。
	commentMvID      = 14514682
	stablePlaylistID = 2219770152
)

type smokeCase struct {
	name string
	call func(ctx context.Context) (proto.Message, error)
	// check 可选:除「非空消息」外的具体断言。
	check func(t *testing.T, resp proto.Message)
}

// knownEmpty 登记目前已知对匿名调用返回空的 RPC(用例名 → 原因):只记录、不失败。
// 显式登记而不是删掉用例——这样它不会悄悄被遗忘;问题修复后(不再为空)会提示移除登记。
var knownEmpty = map[string]string{
	"Playlist.RelatedPlaylistRecommend": "匿名调用时上游 /eapi/playlist/detail/rcmd/get 对所有歌单都返回 data=null(可能需要登录态或接口已变),见 #78",
	"Auth.LoginStatus":                  "匿名调用时上游不返回账号信息,空响应即「未登录」语义,是预期行为",
}

func TestUpstreamSmoke(t *testing.T) {
	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithTimeout(20*time.Second))
	song, album, artist := NewSongServer(eng), NewAlbumServer(eng), NewArtistServer(eng)
	playlist, user := NewPlaylistServer(eng), NewUserServer(eng)
	recommend, search := NewRecommendServer(eng), NewSearchServer(eng)
	authsrv := NewAuthServer(eng, nil)

	cases := []smokeCase{
		// --- 登录(匿名可验证的子集:取二维码 key / 匿名登录态) ---
		{"Auth.LoginQrcode", func(c context.Context) (proto.Message, error) {
			return authsrv.LoginQrcode(c, &mmpb.LoginQrcodeRequest{})
		}, func(t *testing.T, r proto.Message) {
			require.NotEmpty(t, r.(*mmpb.LoginQrcodeResponse).GetQrcode().GetKey(), "二维码 key 非空")
			require.NotEmpty(t, r.(*mmpb.LoginQrcodeResponse).GetQrcode().GetUrl(), "二维码 URL 非空")
		}},
		{"Auth.LoginStatus", func(c context.Context) (proto.Message, error) {
			return authsrv.LoginStatus(c, &mmpb.LoginStatusRequest{})
		}, nil},
		// --- 歌曲 ---
		{"Song.GetSongDetail", func(c context.Context) (proto.Message, error) {
			return song.GetSongDetail(c, &mmpb.GetSongDetailRequest{SongId: songID})
		}, func(t *testing.T, r proto.Message) {
			require.Equal(t, "海阔天空", r.(*mmpb.GetSongDetailResponse).GetSong().GetName())
		}},
		{"Song.GetLyric", func(c context.Context) (proto.Message, error) {
			return song.GetLyric(c, &mmpb.GetLyricRequest{SongId: songID})
		}, func(t *testing.T, r proto.Message) {
			require.NotEmpty(t, r.(*mmpb.GetLyricResponse).GetLyric().GetLrc())
		}},
		{"Song.GetSongURL", func(c context.Context) (proto.Message, error) {
			return song.GetSongURL(c, &mmpb.GetSongURLRequest{SongId: songID})
		}, nil},
		{"Song.CheckAvailable", func(c context.Context) (proto.Message, error) {
			return song.CheckAvailable(c, &mmpb.CheckAvailableRequest{SongId: songID})
		}, nil},
		{"Song.QualityDetail", func(c context.Context) (proto.Message, error) {
			return song.QualityDetail(c, &mmpb.QualityDetailRequest{SongId: songID})
		}, nil},
		{"Song.LikeCount", func(c context.Context) (proto.Message, error) {
			return song.LikeCount(c, &mmpb.LikeCountRequest{SongId: songID})
		}, nil},
		{"Song.SimilarSongs", func(c context.Context) (proto.Message, error) {
			return song.SimilarSongs(c, &mmpb.SimilarSongsRequest{SongId: songID, Limit: 3})
		}, nil},

		// --- 专辑 ---
		{"Album.GetAlbum", func(c context.Context) (proto.Message, error) {
			return album.GetAlbum(c, &mmpb.GetAlbumRequest{AlbumId: albumID})
		}, func(t *testing.T, r proto.Message) {
			resp := r.(*mmpb.GetAlbumResponse)
			require.Equal(t, "海阔天空", resp.GetAlbum().GetName())
			require.NotEmpty(t, resp.GetSongs())
		}},
		{"Album.NewestAlbums", func(c context.Context) (proto.Message, error) {
			return album.NewestAlbums(c, &mmpb.NewestAlbumsRequest{})
		}, nil},
		{"Album.AlbumDynamic", func(c context.Context) (proto.Message, error) {
			return album.AlbumDynamic(c, &mmpb.AlbumDynamicRequest{AlbumId: albumID})
		}, nil},

		// --- 歌手 ---
		{"Artist.GetArtist", func(c context.Context) (proto.Message, error) {
			return artist.GetArtist(c, &mmpb.GetArtistRequest{ArtistId: artistID})
		}, func(t *testing.T, r proto.Message) {
			resp := r.(*mmpb.GetArtistResponse)
			require.Equal(t, "周杰伦", resp.GetArtist().GetName())
			require.NotEmpty(t, resp.GetHotSongs())
		}},
		{"Artist.AllSongs", func(c context.Context) (proto.Message, error) {
			return artist.AllSongs(c, &mmpb.AllSongsRequest{ArtistId: artistID, Limit: 3})
		}, nil},
		{"Artist.TopSongs", func(c context.Context) (proto.Message, error) {
			return artist.TopSongs(c, &mmpb.TopSongsRequest{ArtistId: artistID})
		}, nil},
		{"Artist.Albums", func(c context.Context) (proto.Message, error) {
			return artist.Albums(c, &mmpb.AlbumsRequest{ArtistId: artistID, Limit: 3})
		}, nil},
		{"Artist.Desc", func(c context.Context) (proto.Message, error) {
			return artist.Desc(c, &mmpb.DescRequest{ArtistId: artistID})
		}, nil},
		{"Artist.Fans", func(c context.Context) (proto.Message, error) {
			return artist.Fans(c, &mmpb.FansRequest{ArtistId: artistID})
		}, func(t *testing.T, r proto.Message) {
			require.Positive(t, r.(*mmpb.FansResponse).GetFans())
		}},
		{"Artist.TopArtists", func(c context.Context) (proto.Message, error) {
			return artist.TopArtists(c, &mmpb.TopArtistsRequest{Limit: 3})
		}, nil},

		// --- 歌单 ---
		{"Playlist.GetPlaylist", func(c context.Context) (proto.Message, error) {
			return playlist.GetPlaylist(c, &mmpb.GetPlaylistRequest{PlaylistId: playlistID})
		}, nil},
		{"Playlist.AllTracks", func(c context.Context) (proto.Message, error) {
			return playlist.AllTracks(c, &mmpb.AllTracksRequest{PlaylistId: playlistID, Limit: 3})
		}, nil},
		{"Playlist.BrowseHot", func(c context.Context) (proto.Message, error) {
			return playlist.BrowseHot(c, &mmpb.BrowseHotRequest{Limit: 3})
		}, nil},
		{"Playlist.CatList", func(c context.Context) (proto.Message, error) {
			return playlist.CatList(c, &mmpb.CatListRequest{})
		}, nil},
		{"Playlist.HighQuality", func(c context.Context) (proto.Message, error) {
			return playlist.HighQuality(c, &mmpb.HighQualityRequest{Limit: 3})
		}, nil},
		{"Playlist.HighQualityTags", func(c context.Context) (proto.Message, error) {
			return playlist.HighQualityTags(c, &mmpb.HighQualityTagsRequest{})
		}, nil},
		{"Playlist.Subscribers", func(c context.Context) (proto.Message, error) {
			return playlist.Subscribers(c, &mmpb.SubscribersRequest{PlaylistId: playlistID, Limit: 3})
		}, nil},
		{"Playlist.SimilarPlaylists", func(c context.Context) (proto.Message, error) {
			return playlist.SimilarPlaylists(c, &mmpb.SimilarPlaylistsRequest{SongId: songID, Limit: 3})
		}, nil},
		{"Playlist.RelatedPlaylistRecommend", func(c context.Context) (proto.Message, error) {
			return playlist.RelatedPlaylistRecommend(c, &mmpb.RelatedPlaylistRecommendRequest{PlaylistId: playlistID})
		}, nil},

		// --- 推荐 / 搜索 ---
		{"Recommend.RecommendPlaylists", func(c context.Context) (proto.Message, error) {
			return recommend.RecommendPlaylists(c, &mmpb.RecommendPlaylistsRequest{Limit: 3})
		}, nil},
		{"Recommend.RecommendNewSongs", func(c context.Context) (proto.Message, error) {
			return recommend.RecommendNewSongs(c, &mmpb.RecommendNewSongsRequest{Limit: 3})
		}, nil},
		{"Search.Search", func(c context.Context) (proto.Message, error) {
			return search.Search(c, &mmpb.SearchRequest{Keyword: "周杰伦", Limit: 3})
		}, nil},
		{"Search.Suggest", func(c context.Context) (proto.Message, error) {
			return search.Suggest(c, &mmpb.SuggestRequest{Keyword: "周杰"})
		}, nil},
		{"Search.Hot", func(c context.Context) (proto.Message, error) {
			return search.Hot(c, &mmpb.HotRequest{})
		}, nil},
		{"Search.DefaultKeyword", func(c context.Context) (proto.Message, error) {
			return search.DefaultKeyword(c, &mmpb.DefaultKeywordRequest{})
		}, nil},

		// --- 用户 ---
		{"User.Detail", func(c context.Context) (proto.Message, error) {
			return user.Detail(c, &mmpb.DetailRequest{UserId: userID})
		}, func(t *testing.T, r proto.Message) {
			require.Equal(t, int64(userID), r.(*mmpb.DetailResponse).GetUser().GetUserId())
			require.NotEmpty(t, r.(*mmpb.DetailResponse).GetUser().GetNickname())
		}},
		{"User.Follows", func(c context.Context) (proto.Message, error) {
			return user.Follows(c, &mmpb.FollowsRequest{UserId: userID, Limit: 3})
		}, func(t *testing.T, r proto.Message) {
			require.NotEmpty(t, r.(*mmpb.FollowsResponse).GetFollows())
		}},
		{"User.Followeds", func(c context.Context) (proto.Message, error) {
			return user.Followeds(c, &mmpb.FollowedsRequest{UserId: userID, Limit: 3})
		}, nil},
		{"User.UserPlaylist", func(c context.Context) (proto.Message, error) {
			return user.UserPlaylist(c, &mmpb.UserPlaylistRequest{UserId: userID, Limit: 3})
		}, nil},
		{"User.DetailByName", func(c context.Context) (proto.Message, error) {
			return user.DetailByName(c, &mmpb.DetailByNameRequest{Nickname: "网易云音乐"})
		}, nil},

		// --- 评论(#28 Phase A:proto 只落消息、无 service rpc,直连 endpoint;
		//     第二阶段建模 CommentService 后改走 service 方法) ---
		{"Comment.List(song)", func(c context.Context) (proto.Message, error) {
			return engine.Execute(eng, c, commentendpoint.List, &mmpb.CommentListRequest{
				ResourceType: mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_SONG, ResourceId: songID, PageSize: 3,
			})
		}, func(t *testing.T, r proto.Message) {
			require.NotEmpty(t, r.(*mmpb.CommentListResponse).GetComments())
		}},
		{"Comment.List(playlist)", func(c context.Context) (proto.Message, error) {
			return engine.Execute(eng, c, commentendpoint.List, &mmpb.CommentListRequest{
				ResourceType: mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_PLAYLIST, ResourceId: playlistID, PageSize: 3,
			})
		}, func(t *testing.T, r proto.Message) {
			require.NotEmpty(t, r.(*mmpb.CommentListResponse).GetComments())
		}},
		{"Comment.List(album)", func(c context.Context) (proto.Message, error) {
			return engine.Execute(eng, c, commentendpoint.List, &mmpb.CommentListRequest{
				ResourceType: mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_ALBUM, ResourceId: albumID, PageSize: 3,
			})
		}, func(t *testing.T, r proto.Message) {
			require.NotEmpty(t, r.(*mmpb.CommentListResponse).GetComments())
		}},
		{"Comment.List(mv)", func(c context.Context) (proto.Message, error) {
			return engine.Execute(eng, c, commentendpoint.List, &mmpb.CommentListRequest{
				ResourceType: mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_MV, ResourceId: commentMvID, PageSize: 3,
			})
		}, func(t *testing.T, r proto.Message) {
			require.NotEmpty(t, r.(*mmpb.CommentListResponse).GetComments())
		}},
		{"Comment.Hot(song)", func(c context.Context) (proto.Message, error) {
			return engine.Execute(eng, c, commentendpoint.Hot, &mmpb.HotCommentsRequest{
				ResourceType: mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_SONG, ResourceId: songID, Limit: 3,
			})
		}, func(t *testing.T, r proto.Message) {
			require.NotEmpty(t, r.(*mmpb.HotCommentsResponse).GetHotComments())
		}},
		{"Comment.Hot(playlist)", func(c context.Context) (proto.Message, error) {
			// 不用热歌榜(3778678):榜单歌单的热评随榜单更新清空(total 归零),
			// 2026-10 实测同日先有 1450 条后清空;用稳定的普通歌单(仓库夹具同款)。
			return engine.Execute(eng, c, commentendpoint.Hot, &mmpb.HotCommentsRequest{
				ResourceType: mmpb.CommentResourceType_COMMENT_RESOURCE_TYPE_PLAYLIST, ResourceId: stablePlaylistID, Limit: 3,
			})
		}, func(t *testing.T, r proto.Message) {
			require.NotEmpty(t, r.(*mmpb.HotCommentsResponse).GetHotComments())
		}},
		// --- MV(#32 第一阶段)---
		// 只有 endpoint 声明、无 service 方法,直接经 engine.Execute 冒烟
		// (第二阶段 MvService 落地后换回 server 方法,与本文件其余用例一致)。
		// SubscribedMvs(收藏 MV 列表)需登录态,不在此列;上游对匿名调用
		// 恒返回空列表,带 cookie 的真实验证由 CLI `kite mv subscribed` 承接。
		{"Mv.GetMv", func(c context.Context) (proto.Message, error) {
			return engine.Execute(eng, c, mvendpoint.Detail, &mmpb.GetMvRequest{MvId: mvID})
		}, func(t *testing.T, r proto.Message) {
			mv := r.(*mmpb.GetMvResponse).GetMv()
			require.Equal(t, "广岛之恋", mv.GetName())
			require.NotEmpty(t, mv.GetUrls(), "多清晰度播放地址为空——brs 解析可能与上游不符")
		}},
		{"Mv.TopMvs", func(c context.Context) (proto.Message, error) {
			return engine.Execute(eng, c, mvendpoint.Top, &mmpb.TopMvsRequest{})
		}, func(t *testing.T, r proto.Message) {
			mvs := r.(*mmpb.TopMvsResponse).GetMvs()
			require.NotEmpty(t, mvs, "MV 排行榜为空——解码器可能与上游现行响应形状不符")
			require.NotEmpty(t, mvs[0].GetName())
			require.NotEmpty(t, mvs[0].GetPublishTime(), "发行日期为空说明未从嵌套 mv.publishTime 取值")
			require.Positive(t, mvs[0].GetPlayCount())
		}},
		{"Mv.SimilarMvs", func(c context.Context) (proto.Message, error) {
			return engine.Execute(eng, c, mvendpoint.Similar, &mmpb.SimilarMvsRequest{MvId: mvID})
		}, func(t *testing.T, r proto.Message) {
			mvs := r.(*mmpb.SimilarMvsResponse).GetMvs()
			require.NotEmpty(t, mvs, "相关 MV 为空——解码器可能与上游现行响应形状不符")
			require.NotEmpty(t, mvs[0].GetName())
			require.Positive(t, mvs[0].GetDurationMs(), "时长为 0 说明 duration 字段名与上游不符")
		}},
		// --- 排行榜(#29 第一阶段)---
		// 只有 endpoint 声明、无 service 方法,直接经 engine.Execute 冒烟
		// (第二阶段 ToplistService 落地后换回 server 方法,与本文件其余用例一致)。
		{"Toplist.List", func(c context.Context) (proto.Message, error) {
			return engine.Execute(eng, c, toplistendpoint.List, &mmpb.ToplistsRequest{})
		}, func(t *testing.T, r proto.Message) {
			tl := r.(*mmpb.ToplistsResponse).GetToplists()
			require.NotEmpty(t, tl, "榜单列表为空——解码器可能与上游现行响应形状不符")
			require.NotEmpty(t, tl[0].GetName())
			require.Positive(t, tl[0].GetTrackCount())
		}},
		{"Toplist.Detail", func(c context.Context) (proto.Message, error) {
			return engine.Execute(eng, c, toplistendpoint.Detail, &mmpb.ToplistDetailRequest{})
		}, func(t *testing.T, r proto.Message) {
			tl := r.(*mmpb.ToplistDetailResponse).GetToplists()
			require.NotEmpty(t, tl, "榜单详情为空——解码器可能与上游现行响应形状不符")
			withPreview := 0
			for _, item := range tl {
				if len(item.GetPreviewTracks()) > 0 {
					withPreview++
				}
			}
			require.NotZero(t, withPreview, "没有任何榜单带前三歌曲概要——概要字段名可能与上游不符")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := tc.call(ctx)
			require.NoError(t, err, "对真实上游调用失败")
			if reason, ok := knownEmpty[tc.name]; ok {
				if proto.Size(resp) == 0 {
					t.Logf("已知为空(%s)", reason)
					return
				}
				t.Fatalf("已登记为「已知为空」的 RPC 现在有内容了——请从 knownEmpty 移除登记(原登记: %s)", reason)
			}
			// 「成功但返回空」:解码器与上游响应不符时,宽松解析会静默得到零值。
			require.NotZero(t, proto.Size(resp), "响应是空消息——解码器可能与上游现行响应形状不符")
			if tc.check != nil {
				tc.check(t, resp)
			}
			time.Sleep(300 * time.Millisecond) // 对上游客气一点
		})
	}
}
