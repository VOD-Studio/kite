// Package engine 两条执行路径的特征化测试。
//
// engine 对外有两族入口:
//   - 会话池路径:RawDo / RawDoWithCookie(Execute 走这条)——只有 AuthLoggedIn 的接口才带凭据,
//     ctx 里的 cookie 优先,否则取会话池;匿名接口不带任何用户 cookie;池里取出的 session 失效时上报。
//   - ctx cookie 路径:RawDoWithCookieAndInput(ExecuteWithCookie 走这条)——无条件使用 ctx 里的 cookie
//     (哪怕是匿名接口),从不碰会话池,也不上报。
//
// 两条路径共享「加密分发 / 熔断 / 重试 / 错误映射」,只在「凭据怎么来」上刻意不同。
// 本文件把这些行为钉死:任何重构(合并重复实现)都必须让它保持绿——不允许悄悄抹掉差异,
// 也不允许悄悄让共享部分分叉。
package engine_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	merrors "github.com/VOD-Studio/kite/errors"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// wireReq 是上游看到的一次请求的形状(只保留与加密随机数无关的部分)。
type wireReq struct {
	Method      string
	Path        string
	ContentType string
	Query       string   // 排序后的 query
	FormKeys    []string // form 请求体的字段名(排序)
	JSONBody    string   // JSON 请求体原文
	Cookie      string
}

// wireUpstream 记录上游收到的每个请求,并按配置应答。
type wireUpstream struct {
	srv *httptest.Server

	mu   sync.Mutex
	reqs []wireReq

	calls atomic.Int32

	// 应答配置(测试开始前设置)。
	status    int    // 默认 200
	body      string // 默认 {"code":200}
	setCookie string // 非空则带 Set-Cookie: MUSIC_U=<setCookie>
}

func newWireUpstream(t *testing.T) *wireUpstream {
	t.Helper()
	u := &wireUpstream{status: http.StatusOK, body: `{"code":200}`}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.calls.Add(1)
		raw, _ := io.ReadAll(r.Body)
		wr := wireReq{
			Method:      r.Method,
			Path:        r.URL.Path,
			ContentType: r.Header.Get("Content-Type"),
			Cookie:      r.Header.Get("Cookie"),
		}
		if q := r.URL.Query(); len(q) > 0 {
			keys := make([]string, 0, len(q))
			for k := range q {
				keys = append(keys, k+"="+q.Get(k))
			}
			sort.Strings(keys)
			wr.Query = strings.Join(keys, "&")
		}
		switch {
		case strings.HasPrefix(wr.ContentType, "application/x-www-form-urlencoded"):
			if form, err := url.ParseQuery(string(raw)); err == nil {
				for k := range form {
					wr.FormKeys = append(wr.FormKeys, k)
				}
				sort.Strings(wr.FormKeys)
			}
		case strings.HasPrefix(wr.ContentType, "application/json"):
			wr.JSONBody = string(raw)
		}
		u.mu.Lock()
		u.reqs = append(u.reqs, wr)
		u.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if u.setCookie != "" {
			http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: u.setCookie, Path: "/"}) //nolint:gosec // 测试上游,模拟网易云登录响应的 Set-Cookie
		}
		w.WriteHeader(u.status)
		_, _ = w.Write([]byte(u.body))
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *wireUpstream) last(t *testing.T) wireReq {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	require.NotEmpty(t, u.reqs, "上游没有收到请求")
	return u.reqs[len(u.reqs)-1]
}

// entry 把三个原始入口统一成同一签名,便于对同一场景逐个入口断言。
type entry struct {
	name string
	// ctxPolicy 为 true 表示 ctx cookie 路径(RawDoWithCookieAndInput),false 为会话池路径。
	ctxPolicy bool
	do        func(e *engine.Engine, ctx context.Context, meta engine.Meta, params map[string]any) (json.RawMessage, string, error)
}

var entries = []entry{
	{"RawDo", false, func(e *engine.Engine, ctx context.Context, m engine.Meta, p map[string]any) (json.RawMessage, string, error) {
		raw, err := e.RawDo(ctx, m, p)
		return raw, "", err
	}},
	{"RawDoWithCookie", false, func(e *engine.Engine, ctx context.Context, m engine.Meta, p map[string]any) (json.RawMessage, string, error) {
		return e.RawDoWithCookie(ctx, m, p)
	}},
	{"RawDoWithCookieAndInput", true, func(e *engine.Engine, ctx context.Context, m engine.Meta, p map[string]any) (json.RawMessage, string, error) {
		return e.RawDoWithCookieAndInput(ctx, m, p)
	}},
}

// 加密分发:同一个 Meta 经三个入口,上游看到的请求形状必须完全一致,
// 并且每种加密方式有各自确定的线上形态。
func TestWire_CryptoDispatchIsIdenticalAcrossEntries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		meta engine.Meta
		want wireReq // 只断言稳定字段
	}{
		{"weapi", engine.Meta{Path: "/weapi/x/y", Method: "POST", Crypto: engine.CryptoWeAPI},
			wireReq{Method: "POST", Path: "/weapi/x/y", ContentType: "application/x-www-form-urlencoded", FormKeys: []string{"encSecKey", "params"}}},
		{"eapi", engine.Meta{Path: "/eapi/x/y", Method: "POST", Crypto: engine.CryptoEAPI},
			wireReq{Method: "POST", Path: "/eapi/x/y", ContentType: "application/x-www-form-urlencoded", FormKeys: []string{"params"}}},
		{"无加密 GET", engine.Meta{Path: "/api/g", Method: "GET", Crypto: engine.CryptoNone},
			wireReq{Method: "GET", Path: "/api/g", Query: "id=7"}},
		{"无加密 POST(JSON)", engine.Meta{Path: "/api/p", Method: "POST", Crypto: engine.CryptoNone},
			wireReq{Method: "POST", Path: "/api/p", ContentType: "application/json", JSONBody: `{"id":7}`}},
		{"未知加密方式回落 weapi", engine.Meta{Path: "/weapi/z", Method: "POST", Crypto: engine.CryptoMethod(99)},
			wireReq{Method: "POST", Path: "/weapi/z", ContentType: "application/x-www-form-urlencoded", FormKeys: []string{"encSecKey", "params"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var shapes []wireReq
			for _, en := range entries {
				up := newWireUpstream(t)
				eng := engine.New(engine.WithBaseURL(up.srv.URL))
				_, _, err := en.do(eng, context.Background(), tt.meta, map[string]any{"id": 7})
				require.NoError(t, err, en.name)
				got := up.last(t)
				got.Cookie = "" // 凭据另有专门测试
				shapes = append(shapes, got)

				require.Equal(t, tt.want.Method, got.Method, en.name)
				require.Equal(t, tt.want.Path, got.Path, en.name)
				require.Equal(t, tt.want.Query, got.Query, en.name)
				require.Equal(t, tt.want.JSONBody, got.JSONBody, en.name)
				require.Equal(t, tt.want.FormKeys, got.FormKeys, en.name)
				require.True(t, strings.HasPrefix(got.ContentType, tt.want.ContentType), "%s: Content-Type=%q", en.name, got.ContentType)
			}
			for i := 1; i < len(shapes); i++ {
				require.Equal(t, shapes[0], shapes[i], "入口 %s 与 %s 发往上游的请求不一致", entries[0].name, entries[i].name)
			}
		})
	}
}

// 凭据策略矩阵:这是两条路径刻意不同的地方,逐格钉死。
func TestCredential_PolicyMatrix(t *testing.T) {
	t.Parallel()
	const ctxCookie, poolCookie = "MUSIC_U=ctx", "MUSIC_U=pool"

	type row struct {
		name      string
		auth      session.AuthRequirement
		ctxCookie bool
		pool      bool

		// 期望(会话池路径)
		poolWantErr    error
		poolWantCalls  int32
		poolWantCookie string // 上游 Cookie 头应包含;"" 表示不应带任何用户 cookie
		// 期望(ctx cookie 路径)
		ctxWantCalls  int32
		ctxWantCookie string
	}
	rows := []row{
		{"匿名接口 + ctx cookie", session.AuthAnonymous, true, false,
			nil, 1, "", // 池路径:匿名接口不带用户 cookie
			1, ctxCookie}, // ctx 路径:无条件带
		{"登录接口 + ctx cookie", session.AuthLoggedIn, true, false,
			nil, 1, ctxCookie,
			1, ctxCookie},
		{"登录接口 + 只有会话池", session.AuthLoggedIn, false, true,
			nil, 1, poolCookie, // 池路径:取池
			1, ""}, // ctx 路径:不碰池,发空 cookie 给上游
		{"登录接口 + ctx cookie 与会话池并存", session.AuthLoggedIn, true, true,
			nil, 1, ctxCookie, // ctx 优先
			1, ctxCookie},
		{"登录接口 + 什么凭据都没有", session.AuthLoggedIn, false, false,
			merrors.ErrUnauthorized, 0, "", // 池路径:本地拒绝,不打上游
			1, ""}, // ctx 路径:照常请求(由上游判定)
		{"可选登录 + ctx cookie", session.AuthOptional, true, false,
			nil, 1, ctxCookie, // 池路径:带着 ctx cookie
			1, ctxCookie},
		{"可选登录 + 只有会话池", session.AuthOptional, false, true,
			nil, 1, "", // 池路径:不取池,按匿名请求
			1, ""},
		{"可选登录 + 什么凭据都没有", session.AuthOptional, false, false,
			nil, 1, "", // 池路径:不报未授权,按匿名请求
			1, ""},
	}

	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			for _, en := range entries {
				up := newWireUpstream(t)
				opts := []engine.Option{engine.WithBaseURL(up.srv.URL)}
				if r.pool {
					opts = append(opts, engine.WithSessions(&fixedSessions{sess: &session.Session{UserID: "42", Cookie: poolCookie}}))
				}
				eng := engine.New(opts...)
				ctx := context.Background()
				if r.ctxCookie {
					ctx = engine.WithCookie(ctx, ctxCookie)
				}
				meta := engine.Meta{Path: "/weapi/x", Method: "POST", Crypto: engine.CryptoWeAPI, Auth: r.auth}

				_, _, err := en.do(eng, ctx, meta, map[string]any{})

				wantCalls, wantCookie, wantErr := r.poolWantCalls, r.poolWantCookie, r.poolWantErr
				if en.ctxPolicy {
					wantCalls, wantCookie, wantErr = r.ctxWantCalls, r.ctxWantCookie, nil
				}
				if wantErr != nil {
					require.ErrorIs(t, err, wantErr, en.name)
				} else {
					require.NoError(t, err, en.name)
				}
				require.Equal(t, wantCalls, up.calls.Load(), "%s: 上游调用次数", en.name)
				if wantCalls == 0 {
					continue
				}
				got := up.last(t).Cookie
				if wantCookie == "" {
					require.Equal(t, "__remember_me=true", got, "%s: 不应带任何用户 cookie", en.name)
				} else {
					require.Contains(t, got, wantCookie, en.name)
				}
				if wantCookie != poolCookie {
					require.NotContains(t, got, poolCookie, "%s: 不该用会话池的 cookie", en.name)
				}
			}
		})
	}
}

// Set-Cookie 回传:只有 weapi 响应的 Set-Cookie 会被带回(登录类接口靠它拿 session);
// eapi 与无加密请求不回传。RawDo 不暴露 Set-Cookie。
func TestSetCookie_Propagation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		meta engine.Meta
		want string
	}{
		{"weapi 回传", engine.Meta{Path: "/weapi/login", Method: "POST", Crypto: engine.CryptoWeAPI}, "MUSIC_U=fresh"},
		{"eapi 不回传", engine.Meta{Path: "/eapi/x", Method: "POST", Crypto: engine.CryptoEAPI}, ""},
		{"无加密 GET 不回传", engine.Meta{Path: "/api/g", Method: "GET", Crypto: engine.CryptoNone}, ""},
		{"无加密 POST 不回传", engine.Meta{Path: "/api/p", Method: "POST", Crypto: engine.CryptoNone}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, en := range entries {
				up := newWireUpstream(t)
				up.setCookie = "fresh"
				eng := engine.New(engine.WithBaseURL(up.srv.URL))
				_, sc, err := en.do(eng, context.Background(), tt.meta, map[string]any{"id": 1})
				require.NoError(t, err, en.name)
				if en.name == "RawDo" {
					continue // RawDo 的签名里没有 Set-Cookie
				}
				require.Equal(t, tt.want, sc, en.name)
			}
		})
	}
}

// 会话池里取出的 session 失效(未授权)时必须上报,且只上报「本次实际用的那个」;
// ctx 路径从不上报,ctx 里自带 cookie 时也不能怪罪池里的 session。
func TestPoolFailureReporting(t *testing.T) {
	t.Parallel()
	loggedIn := engine.Meta{Path: "/weapi/x", Method: "POST", Crypto: engine.CryptoWeAPI, Auth: session.AuthLoggedIn}

	run := func(t *testing.T, en entry, withCtxCookie bool) int32 {
		t.Helper()
		up := newWireUpstream(t)
		up.body = `{"code":301,"msg":"需要登录"}`
		pool := &fixedSessions{sess: &session.Session{UserID: "42", Cookie: "MUSIC_U=pool"}}
		eng := engine.New(engine.WithBaseURL(up.srv.URL), engine.WithSessions(pool))
		ctx := context.Background()
		if withCtxCookie {
			ctx = engine.WithCookie(ctx, "MUSIC_U=ctx")
		}
		_, _, err := en.do(eng, ctx, loggedIn, map[string]any{})
		require.ErrorIs(t, err, merrors.ErrUnauthorized, en.name)
		return pool.failed.Load()
	}

	for _, en := range entries {
		t.Run(en.name, func(t *testing.T) {
			t.Parallel()
			if en.ctxPolicy {
				require.Zero(t, run(t, en, false), "ctx 路径从不上报")
				require.Zero(t, run(t, en, true), "ctx 路径从不上报")
				return
			}
			require.Equal(t, int32(1), run(t, en, false), "用了池里的 session,失效就该上报")
			require.Zero(t, run(t, en, true), "用的是 ctx 自带的 cookie,不能怪罪池里的 session")
		})
	}
}

// 重试与熔断是两条路径共享的:上游 5xx 重试到上限(首次 + 3 次),连续失败累积到阈值后熔断器打开,
// 之后不再打上游。
func TestRetryAndBreaker_SharedByAllEntries(t *testing.T) {
	t.Parallel()
	for _, en := range entries {
		t.Run(en.name, func(t *testing.T) {
			t.Parallel()
			up := newWireUpstream(t)
			up.status = http.StatusServiceUnavailable
			eng := engine.New(engine.WithBaseURL(up.srv.URL))
			meta := engine.Meta{Path: "/weapi/x", Method: "POST", Crypto: engine.CryptoWeAPI}

			_, _, err := en.do(eng, context.Background(), meta, map[string]any{})
			require.ErrorIs(t, err, merrors.ErrUpstreamUnavailable)
			require.Equal(t, int32(4), up.calls.Load(), "首次 + 3 次重试")

			_, _, err = en.do(eng, context.Background(), meta, map[string]any{})
			require.ErrorIs(t, err, merrors.ErrUpstreamUnavailable)
			require.Equal(t, int32(8), up.calls.Load())

			// 连续失败已超过阈值:熔断器打开,不再请求上游。
			_, _, err = en.do(eng, context.Background(), meta, map[string]any{})
			require.ErrorIs(t, err, engine.ErrCircuitOpen)
			require.Equal(t, int32(8), up.calls.Load(), "熔断后不应再打上游")
		})
	}
}
