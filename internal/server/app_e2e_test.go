package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

const testToken = "test-token-123"

// startApp 用真实的 NewApp(真实 TCP 监听、gateway、拦截器链)起服务,监听系统分配的回环端口。
// 上游网易云由 httptest 模拟(恒返回业务码 301 → LOGIN_REQUIRED),返回值可读取上游被命中的次数:
// 用它证明「鉴权失败的请求没有走到业务层」。
func startApp(t *testing.T, token string) (app *App, upstreamHits *atomic.Int32) {
	t.Helper()
	hits := &atomic.Int32{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":301}`))
	}))
	t.Cleanup(up.Close)

	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(up.URL))
	app, err := NewApp(Options{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0", AuthTokens: single(token)}, eng, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	})
	return app, hits
}

func dialApp(t *testing.T, app *App) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(app.GRPCAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func withToken(ctx context.Context, token string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

func bg(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestApp_GRPC_Auth(t *testing.T) {
	app, hits := startApp(t, testToken)
	client := mmpb.NewSongServiceClient(dialApp(t, app))
	req := &mmpb.GetSongDetailRequest{SongId: 1}

	t.Run("无令牌 → SERVER_AUTH_FAILED,且没触达上游", func(t *testing.T) {
		_, err := client.GetSongDetail(bg(t), req)
		require.Equal(t, codes.Unauthenticated, status.Code(err))
		require.Equal(t, ReasonServerAuthFailed, errorInfoOf(t, err).GetReason())
		require.Zero(t, hits.Load())
	})
	t.Run("令牌错误,即使带了 cookie 也不触达上游", func(t *testing.T) {
		ctx := metadata.AppendToOutgoingContext(bg(t), "x-netease-cookie", "MUSIC_U=x")
		_, err := client.GetSongDetail(withToken(ctx, "wrong"), req)
		require.Equal(t, ReasonServerAuthFailed, errorInfoOf(t, err).GetReason())
		require.Zero(t, hits.Load())
	})
	t.Run("令牌正确 → 走到业务层(上游 301 = LOGIN_REQUIRED,与服务鉴权失败可区分)", func(t *testing.T) {
		_, err := client.GetSongDetail(withToken(bg(t), testToken), req)
		require.Equal(t, codes.Unauthenticated, status.Code(err))
		require.Equal(t, ReasonLoginRequired, errorInfoOf(t, err).GetReason())
		require.Positive(t, hits.Load())
	})
}

func TestApp_GRPC_NoAuthConfigured_Loopback(t *testing.T) {
	app, hits := startApp(t, "")
	client := mmpb.NewSongServiceClient(dialApp(t, app))

	_, err := client.GetSongDetail(bg(t), &mmpb.GetSongDetailRequest{SongId: 1})
	require.Equal(t, ReasonLoginRequired, errorInfoOf(t, err).GetReason(), "未配置鉴权的回环服务应直接放行")
	require.Positive(t, hits.Load())
}

func TestApp_GRPC_Health(t *testing.T) {
	app, _ := startApp(t, testToken)
	hc := healthpb.NewHealthClient(dialApp(t, app))

	// 健康检查不需要令牌(编排系统探活不携带凭据)。
	for _, svc := range []string{"", "netease.music.v1.SongService"} {
		resp, err := hc.Check(bg(t), &healthpb.HealthCheckRequest{Service: svc})
		require.NoError(t, err, svc)
		require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus(), svc)
	}
}

// reflection 是流式 RPC,unary 拦截器管不到;没有 stream 拦截器它会成为无需凭据的入口。
func TestApp_Reflection_RequiresAuth(t *testing.T) {
	app, _ := startApp(t, testToken)
	rc := reflectionpb.NewServerReflectionClient(dialApp(t, app))

	list := func(ctx context.Context) error {
		stream, err := rc.ServerReflectionInfo(ctx)
		if err != nil {
			return err
		}
		if err := stream.Send(&reflectionpb.ServerReflectionRequest{
			MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{ListServices: ""},
		}); err != nil && err != io.EOF {
			return err
		}
		_, err = stream.Recv()
		return err
	}

	err := list(bg(t))
	require.Equal(t, codes.Unauthenticated, status.Code(err), "无令牌的 reflection 必须被拒: %v", err)
	require.Equal(t, ReasonServerAuthFailed, errorInfoOf(t, err).GetReason())

	require.NoError(t, list(withToken(bg(t), testToken)))
}

func httpGet(t *testing.T, url string, header http.Header) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(bg(t), http.MethodGet, url, nil)
	require.NoError(t, err)
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body
}

func reasonOf(t *testing.T, body []byte) string {
	t.Helper()
	var v struct {
		Details []struct {
			Reason string `json:"reason"`
		} `json:"details"`
	}
	require.NoError(t, json.Unmarshal(body, &v), string(body))
	require.NotEmpty(t, v.Details, string(body))
	return v.Details[0].Reason
}

// REST 经 grpc-gateway 转到同一个 gRPC server:Authorization 头必须被原样转发,
// REST 与 gRPC 共用一套鉴权规则。
func TestApp_REST_Auth(t *testing.T) {
	app, hits := startApp(t, testToken)
	url := "http://" + app.HTTPAddr() + "/api/v1/songs/1"

	code, body := httpGet(t, url, nil)
	require.Equal(t, http.StatusUnauthorized, code, string(body))
	require.Equal(t, ReasonServerAuthFailed, reasonOf(t, body))
	require.Zero(t, hits.Load())

	code, body = httpGet(t, url, http.Header{"Authorization": {"Bearer wrong"}})
	require.Equal(t, http.StatusUnauthorized, code)
	require.Equal(t, ReasonServerAuthFailed, reasonOf(t, body))

	code, body = httpGet(t, url, http.Header{"Authorization": {"Bearer " + testToken}})
	require.Equal(t, http.StatusUnauthorized, code, "上游 301 → 401")
	require.Equal(t, ReasonLoginRequired, reasonOf(t, body), "令牌通过后应走到业务层")
	require.Positive(t, hits.Load())
}

func TestApp_REST_Healthz(t *testing.T) {
	app, _ := startApp(t, testToken)

	code, body := httpGet(t, "http://"+app.HTTPAddr()+"/healthz", nil)
	require.Equal(t, http.StatusOK, code, "探活不需要令牌")
	require.JSONEq(t, `{"status":"SERVING"}`, string(body))
}

func TestHealthzHandler_NotServing(t *testing.T) {
	h := health.NewServer()
	h.Shutdown()

	rec := httptest.NewRecorder()
	healthzHandler(h)(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.JSONEq(t, `{"status":"NOT_SERVING"}`, rec.Body.String())
}

func TestNewApp_RefusesExposedWithoutToken(t *testing.T) {
	eng := engine.New(engine.WithCache(cache.Noop{}))

	for _, addrs := range [][2]string{
		{"0.0.0.0:0", "127.0.0.1:0"},
		{"127.0.0.1:0", ":0"},
	} {
		app, err := NewApp(Options{GRPCAddr: addrs[0], HTTPAddr: addrs[1]}, eng, nil)
		require.Error(t, err, addrs)
		require.Nil(t, app)
	}
}

// 端口被占用必须从 NewApp 返回错误(过去 HTTP 端口冲突只写一行日志,进程还活着但 REST 不可用),
// 且已占用的另一个端口要被释放。
func TestNewApp_ListenFailure(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = busy.Close() }()

	free, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	freeAddr := free.Addr().String()
	require.NoError(t, free.Close())

	eng := engine.New(engine.WithCache(cache.Noop{}))
	_, err = NewApp(Options{GRPCAddr: freeAddr, HTTPAddr: busy.Addr().String()}, eng, nil)
	require.ErrorContains(t, err, "listen http")

	again, err := net.Listen("tcp", freeAddr)
	require.NoError(t, err, "HTTP 失败后 gRPC 监听应被释放")
	_ = again.Close()
}

func TestApp_Shutdown(t *testing.T) {
	app, _ := startApp(t, testToken)
	grpcAddr, httpAddr := app.GRPCAddr(), app.HTTPAddr()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))

	for _, addr := range []string{grpcAddr, httpAddr} {
		_, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		require.Error(t, err, "关闭后 %s 不应再接受连接", addr)
	}
}

// 关闭流程第一步必须把健康状态置为 NOT_SERVING,让 LB / 编排系统先摘流再排空。
// 用 Watch 流观察状态变化:去掉 health.Shutdown() 时流只会以错误结束。
func TestApp_Shutdown_MarksNotServingFirst(t *testing.T) {
	app, _ := startApp(t, testToken)
	hc := healthpb.NewHealthClient(dialApp(t, app))

	watch, err := hc.Watch(bg(t), &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	first, err := watch.Recv()
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, first.GetStatus())

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	}()
	next, err := watch.Recv()
	require.NoError(t, err, "关闭时应先收到 NOT_SERVING,而不是流直接断开")
	require.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, next.GetStatus())
}
