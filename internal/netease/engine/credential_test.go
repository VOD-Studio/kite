// Package engine 的登录态解析与缓存隔离测试。
//
// 回归背景:服务端的 GetDailyRecommend / GetPersonalFM / LikedList 声明为 AuthLoggedIn
// 且走 engine.Execute,而 doOnce 只从 session 池取凭据、忽略 interceptor 注入 ctx 的 cookie,
// 在 cmd/server 的装配(无 session 池)下恒返回「登录态失效」;同时它们的缓存 key 不含用户
// 身份,接入共享缓存后会跨用户串数据。
package engine_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// fixedSessions 是只返回一个固定 session 的 SessionStore 测试桩。
type fixedSessions struct {
	sess   *session.Session
	failed atomic.Int32
}

func (f *fixedSessions) GetAvailable(context.Context, session.AuthRequirement) (*session.Session, error) {
	if f.sess == nil {
		return nil, session.ErrNoAvailableSession
	}
	return f.sess, nil
}
func (f *fixedSessions) ReportSuccess(string)                         {}
func (f *fixedSessions) ReportFailure(string, error)                  { f.failed.Add(1) }
func (f *fixedSessions) Save(context.Context, *session.Session) error { return nil }
func (f *fixedSessions) ListAll(context.Context) ([]string, error)    { return nil, nil }

// recordingUpstream 记录上游收到的 Cookie 头与调用次数。
type recordingUpstream struct {
	srv     *httptest.Server
	calls   atomic.Int32
	mu      sync.Mutex
	cookies []string
}

func newRecordingUpstream(t *testing.T) *recordingUpstream {
	t.Helper()
	u := &recordingUpstream{}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.calls.Add(1)
		u.mu.Lock()
		u.cookies = append(u.cookies, r.Header.Get("Cookie"))
		u.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"songs":[{"id":1,"name":"歌"}]}`))
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *recordingUpstream) lastCookie() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.cookies) == 0 {
		return ""
	}
	return u.cookies[len(u.cookies)-1]
}

// loggedInEndpoint 构造一个需要登录态、带缓存策略的 endpoint(缓存 key 刻意不含用户身份,
// 与 recommend:daily / fm:personal 一致)。
func loggedInEndpoint() *engine.Endpoint[*mmpb.GetSongDetailRequest, *mmpb.GetSongDetailResponse] {
	ep := makeSongDetailEndpoint()
	ep.Meta.Auth = session.AuthLoggedIn
	return ep
}

// Execute 对 AuthLoggedIn 接口应使用 ctx 里 interceptor 注入的 cookie,而不是只认 session 池。
func TestExecute_LoggedIn_UsesContextCookie(t *testing.T) {
	t.Parallel()
	up := newRecordingUpstream(t)
	eng := engine.New(engine.WithBaseURL(up.srv.URL)) // 与 cmd/server 一致:无 session 池

	ctx := engine.WithCookie(context.Background(), "MUSIC_U=abc")
	resp, err := engine.Execute(eng, ctx, loggedInEndpoint(), &mmpb.GetSongDetailRequest{SongId: 1})
	require.NoError(t, err)
	require.Equal(t, "歌", resp.GetSong().GetName())
	require.Contains(t, up.lastCookie(), "MUSIC_U=abc", "ctx 里的 cookie 应发往上游")
}

// ctx 没有 cookie 时回落 session 池。
func TestExecute_LoggedIn_FallsBackToSessionPool(t *testing.T) {
	t.Parallel()
	up := newRecordingUpstream(t)
	pool := &fixedSessions{sess: &session.Session{UserID: "42", Cookie: "MUSIC_U=pool"}}
	eng := engine.New(engine.WithBaseURL(up.srv.URL), engine.WithSessions(pool))

	_, err := engine.Execute(eng, context.Background(), loggedInEndpoint(), &mmpb.GetSongDetailRequest{SongId: 1})
	require.NoError(t, err)
	require.Contains(t, up.lastCookie(), "MUSIC_U=pool")
}

// 既无 ctx cookie 也无 session 池:明确的未授权,且不会请求上游。
func TestExecute_LoggedIn_NoCredential(t *testing.T) {
	t.Parallel()
	up := newRecordingUpstream(t)
	eng := engine.New(engine.WithBaseURL(up.srv.URL))

	_, err := engine.Execute(eng, context.Background(), loggedInEndpoint(), &mmpb.GetSongDetailRequest{SongId: 1})
	require.ErrorIs(t, err, merrors.ErrUnauthorized)
	require.Zero(t, up.calls.Load(), "没有凭据时不应请求上游")
}

// 需要登录的接口,缓存必须按凭据隔离:不同用户互不命中,同一用户命中;
// 缓存 key 里不能出现原始 cookie(Redis key 是可被枚举 / 记日志的)。
func TestExecute_LoggedIn_CacheIsolatedPerCredential(t *testing.T) {
	t.Parallel()
	up := newRecordingUpstream(t)
	c := &countingCache{}
	eng := engine.New(engine.WithBaseURL(up.srv.URL), engine.WithCache(c))
	ep := loggedInEndpoint()
	req := &mmpb.GetSongDetailRequest{SongId: 1}

	alice := engine.WithCookie(context.Background(), "MUSIC_U=alice")
	bob := engine.WithCookie(context.Background(), "MUSIC_U=bob")

	_, err := engine.Execute(eng, alice, ep, req)
	require.NoError(t, err)
	_, err = engine.Execute(eng, bob, ep, req)
	require.NoError(t, err)
	require.Equal(t, int32(2), up.calls.Load(), "不同用户不应共享缓存条目")

	_, err = engine.Execute(eng, alice, ep, req)
	require.NoError(t, err)
	require.Equal(t, int32(2), up.calls.Load(), "同一用户第二次应命中缓存")

	c.mu.Lock()
	defer c.mu.Unlock()
	require.Len(t, c.store, 2)
	for k := range c.store {
		require.NotContains(t, k, "alice")
		require.NotContains(t, k, "bob")
		require.False(t, strings.Contains(k, "MUSIC_U"), "缓存 key 不能含原始 cookie: %s", k)
	}
}

// 走 session 池时身份在选 session 之前未知,为避免串数据直接绕过缓存。
func TestExecute_LoggedIn_PoolPathBypassesCache(t *testing.T) {
	t.Parallel()
	up := newRecordingUpstream(t)
	c := &countingCache{}
	pool := &fixedSessions{sess: &session.Session{UserID: "42", Cookie: "MUSIC_U=pool"}}
	eng := engine.New(engine.WithBaseURL(up.srv.URL), engine.WithCache(c), engine.WithSessions(pool))
	ep := loggedInEndpoint()
	req := &mmpb.GetSongDetailRequest{SongId: 1}

	for i := 0; i < 2; i++ {
		_, err := engine.Execute(eng, context.Background(), ep, req)
		require.NoError(t, err)
	}
	require.Equal(t, int32(2), up.calls.Load(), "池路径不应命中缓存")
	require.Zero(t, c.sets, "池路径不应写缓存")
}

// 匿名接口的缓存 key 保持不变(各用户共享),且不会把 ctx 里的 cookie 发给上游。
func TestExecute_Anonymous_CacheSharedAndNoCookieSent(t *testing.T) {
	t.Parallel()
	up := newRecordingUpstream(t)
	c := &countingCache{}
	eng := engine.New(engine.WithBaseURL(up.srv.URL), engine.WithCache(c))
	ep := makeSongDetailEndpoint() // Auth 默认 AuthAnonymous
	req := &mmpb.GetSongDetailRequest{SongId: 1}

	_, err := engine.Execute(eng, engine.WithCookie(context.Background(), "MUSIC_U=alice"), ep, req)
	require.NoError(t, err)
	_, err = engine.Execute(eng, engine.WithCookie(context.Background(), "MUSIC_U=bob"), ep, req)
	require.NoError(t, err)

	require.Equal(t, int32(1), up.calls.Load(), "匿名接口应跨用户共享缓存")
	require.NotContains(t, up.lastCookie(), "MUSIC_U", "匿名接口不应携带用户 cookie")
}
