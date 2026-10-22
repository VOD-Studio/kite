package djradio

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/VOD-Studio/kite/internal/netease/session"
)

// 夹具全部取自真实上游响应(testdata/,2026-10 抓取,仅裁剪列表长度、对象逐字保留),
// 不手写「我以为的形状」:上游改字段名 / 类型时宽松解析会静默得到零值,
// 手写夹具与解码器出自同一个假设,永远发现不了(PR #77 的教训)。
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return b
}

// CateList:categories 是数组(旧社区文档写的是带 sub 的对象,真实上游不是)。
func TestCateList_RealShape(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/weapi/djradio/category/get", CateList.Meta.Path)
	require.Equal(t, session.AuthAnonymous, CateList.Meta.Auth)

	params, err := CateList.MapRequest(&CateListRequest{})
	require.NoError(t, err)
	require.Empty(t, params)

	resp, err := CateList.MapResponse(&CateListRequest{}, readFixture(t, "dj_catelist.json"))
	require.NoError(t, err)
	require.Len(t, resp.Categories, 2)
	require.Equal(t, int64(3), resp.Categories[0].ID)
	require.Equal(t, "情感", resp.Categories[0].Name)
	require.Equal(t, "音乐播客", resp.Categories[1].Name)
}

// Programs:radioId 字符串化进 body;夹具断言解码字段与上游真值一致。
func TestPrograms_RealShape(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/weapi/dj/program/byradio", Programs.Meta.Path)
	require.Equal(t, session.AuthAnonymous, Programs.Meta.Auth)

	req := &ProgramsRequest{RadioID: 792544462, Limit: 5, Offset: 0, Asc: false}
	params, err := Programs.MapRequest(req)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"radioId": "792544462", "limit": int32(5), "offset": int32(0), "asc": false}, params)

	// limit<=0 时上游默认 30。
	p2, err := Programs.MapRequest(&ProgramsRequest{RadioID: 1})
	require.NoError(t, err)
	require.Equal(t, int32(30), p2["limit"])

	resp, err := Programs.MapResponse(req, readFixture(t, "dj_program_byradio.json"))
	require.NoError(t, err)
	require.Equal(t, int64(31), resp.Count)
	require.True(t, resp.More)
	require.Len(t, resp.Programs, 2)

	p := resp.Programs[0]
	require.Equal(t, int64(2066770638), p.ID)
	require.Equal(t, "Ssi那个石家庄人（四合院版本）", p.Name)
	require.Equal(t, int64(22125), p.DurationMs)
	require.Equal(t, int64(31), p.SerialNum)
	require.Equal(t, int64(1587482417278), p.CreateTimeMs)
	require.Equal(t, int64(2791529), p.ListenerCount)
	require.Equal(t, int64(1692), p.LikedCount)
	require.Equal(t, int64(48), p.ShareCount)
	require.Equal(t, int64(209), p.CommentCount)
	require.Equal(t, int64(1442398087), p.MainTrackID)
	require.NotEmpty(t, p.CoverURL)

	// 节目内嵌 radio:主播字段在夹具里是 null,解码不应 panic 且应为 nil。
	require.NotNil(t, p.Radio)
	require.Equal(t, int64(792544462), p.Radio.ID)
	require.Equal(t, "四只烤翅", p.Radio.Name)
	require.Equal(t, "创作翻唱", p.Radio.Category)
	require.Equal(t, "抖音热歌", p.Radio.SecondCategory)
	require.Equal(t, int64(244059), p.Radio.SubCount)
	require.Equal(t, int64(33), p.Radio.ProgramCount)
	require.Nil(t, p.Radio.Anchor)

	// 节目自身的主播(dj)在列表响应里有值。
	require.NotNil(t, p.Anchor)
	require.Equal(t, int64(426366432), p.Anchor.UserID)
	require.Equal(t, "四十只烤翅", p.Anchor.Nickname)
	require.NotEmpty(t, p.Anchor.AvatarURL)
}

// ProgramDetail:顶层 program;夹具断言解码字段与上游真值一致。
func TestProgramDetail_RealShape(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/weapi/dj/program/detail", ProgramDetail.Meta.Path)
	require.Equal(t, session.AuthAnonymous, ProgramDetail.Meta.Auth)

	req := &ProgramDetailRequest{ProgramID: 2066770638}
	params, err := ProgramDetail.MapRequest(req)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"id": "2066770638"}, params)

	resp, err := ProgramDetail.MapResponse(req, readFixture(t, "dj_program_detail.json"))
	require.NoError(t, err)
	require.Equal(t, int64(2066770638), resp.ID)
	require.Equal(t, "Ssi那个石家庄人（四合院版本）", resp.Name)
	require.Equal(t, int64(22125), resp.DurationMs)
	require.Equal(t, int64(1442398087), resp.MainTrackID)
	require.NotNil(t, resp.Anchor)
	require.Equal(t, "四十只烤翅", resp.Anchor.Nickname)
	require.NotNil(t, resp.Radio)
	require.Equal(t, "四只烤翅", resp.Radio.Name)
}

// ProgramDetail:响应缺 program 字段时报错而不是返回 nil 零值。
func TestProgramDetail_MissingProgram(t *testing.T) {
	t.Parallel()

	_, err := ProgramDetail.MapResponse(&ProgramDetailRequest{}, []byte(`{"code":200}`))
	require.ErrorContains(t, err, "program")
}

// Subscribed:AuthLoggedIn + total:true;匿名上游返回 200 + 空 djRadios
// (夹具即匿名真身),空列表是合法解码结果,「有数据」的验证需登录态。
func TestSubscribed_RealShape(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/weapi/djradio/get/subed", Subscribed.Meta.Path)
	require.Equal(t, session.AuthLoggedIn, Subscribed.Meta.Auth)

	params, err := Subscribed.MapRequest(&SubscribedRequest{Limit: 30, Offset: 0})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"limit": int32(30), "offset": int32(0), "total": true}, params)

	// limit<=0 时上游默认 30。
	p2, err := Subscribed.MapRequest(&SubscribedRequest{})
	require.NoError(t, err)
	require.Equal(t, int32(30), p2["limit"])

	resp, err := Subscribed.MapResponse(&SubscribedRequest{}, readFixture(t, "dj_sublist_empty.json"))
	require.NoError(t, err)
	require.Empty(t, resp.DjRadios)
	require.Zero(t, resp.Count)
}
