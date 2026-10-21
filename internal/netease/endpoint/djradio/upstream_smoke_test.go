//go:build upstream

package djradio

// djradio 域的真实上游冒烟(issue #33 第一阶段)。
//
// 与 internal/service/upstream_test.go(PR #77 建立)同一模式:对真实上游逐个调用
// endpoint 声明,断言「调用成功且响应不是空」。djradio 尚未 proto 建模、没有
// service 方法,故登记在本包直接经 engine.Execute 调用,Makefile 的
// upstream-smoke target 一并执行。
//
// 手动运行(需外网,不进 CI):make upstream-smoke

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

const (
	smokeRadioID   = 792544462  // 「四只烤翅」电台(2020 年开播,已完结存档,长期稳定)
	smokeProgramID = 2066770638 // 该电台第 31 期(夹具同源)
)

func TestUpstreamSmokeDjRadio(t *testing.T) {
	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithTimeout(20*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("DjRadio.CateList", func(t *testing.T) {
		resp, err := engine.Execute(eng, ctx, CateList, &CateListRequest{})
		require.NoError(t, err, "对真实上游调用失败")
		require.NotEmpty(t, resp.Categories, "分类列表为空——解码器可能与上游现行响应形状不符")
		require.Equal(t, int64(3), resp.Categories[0].ID, "首个分类应是「情感」(id=3),变了请同步夹具")
		require.NotEmpty(t, resp.Categories[0].Name)
		time.Sleep(300 * time.Millisecond) // 对上游客气一点
	})

	t.Run("DjRadio.Programs", func(t *testing.T) {
		resp, err := engine.Execute(eng, ctx, Programs, &ProgramsRequest{RadioID: smokeRadioID, Limit: 3})
		require.NoError(t, err, "对真实上游调用失败")
		require.NotEmpty(t, resp.Programs, "节目列表为空——解码器可能与上游现行响应形状不符")
		require.Positive(t, resp.Count, "上游应返回总数")
		require.NotEmpty(t, resp.Programs[0].Name)
		require.NotZero(t, resp.Programs[0].MainTrackID, "mainTrackId 为 0——字段名可能与上游不符")
		time.Sleep(300 * time.Millisecond)
	})

	t.Run("DjRadio.ProgramDetail", func(t *testing.T) {
		resp, err := engine.Execute(eng, ctx, ProgramDetail, &ProgramDetailRequest{ProgramID: smokeProgramID})
		require.NoError(t, err, "对真实上游调用失败")
		require.Equal(t, int64(smokeProgramID), resp.ID)
		require.NotEmpty(t, resp.Name, "节目名为空——解码器可能与上游现行响应形状不符")
		require.NotZero(t, resp.MainTrackID)
		require.NotNil(t, resp.Radio, "详情应内嵌所属电台")
		time.Sleep(300 * time.Millisecond)
	})

	// Subscribed:需登录态。匿名(空 cookie,ctx cookie 路径)上游返回 200 + 空 djRadios
	// (2026-10 实测,见 testdata/dj_sublist_empty.json),不是 301——显式登记「已知为空」
	// 而不是删掉用例:它同时验证 path/参数可达(路径错了会是 NOT_FOUND 而非空列表);
	// 若上游某天匿名也给数据,此用例会失败提示移除登记。有数据的验证需登录 cookie,
	// 登记在 issue #33 notes,本冒烟不覆盖。
	t.Run("DjRadio.Subscribed_AnonymousKnownEmpty", func(t *testing.T) {
		resp, err := engine.ExecuteWithCookie(eng, ctx, Subscribed, &SubscribedRequest{Limit: 30})
		require.NoError(t, err, "对真实上游调用失败(匿名调用应返回 200 + 空列表)")
		require.Empty(t, resp.DjRadios, "匿名调用已能拿到订阅列表(上游行为变化?)——请移除本登记并用真实登录态补验证")
		t.Log("已知为空(匿名上游返回 200 + 空 djRadios,需登录态才有数据,见 issue #33 notes)")
	})
}
