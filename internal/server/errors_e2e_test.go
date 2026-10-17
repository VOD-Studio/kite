package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// startE2E 用真实的 newGRPCServer(含拦截器链与全部 service)在 bufconn 上起服务,
// 上游网易云由 httptest 模拟,返回带固定业务码的响应。不监听真实端口。
func startE2E(t *testing.T, upstreamBody string) (mmpb.SongServiceClient, *runtime.ServeMux) {
	t.Helper()

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(upstreamBody))
	}))
	t.Cleanup(up.Close)

	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(up.URL))
	srv, _ := newGRPCServer(eng, session.NoopStore{}, nil)

	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := mmpb.NewSongServiceClient(conn)
	mux := runtime.NewServeMux()
	require.NoError(t, mmpb.RegisterSongServiceHandlerClient(context.Background(), mux, client))
	return client, mux
}

// 经真实 server 装配:未登录映射为 Unauthenticated + ErrorInfo,而不是 Unknown。
func TestE2E_Unauthorized(t *testing.T) {
	client, _ := startE2E(t, `{"code":301,"msg":"需要登录"}`)

	_, err := client.GetSongDetail(context.Background(), &mmpb.GetSongDetailRequest{SongId: 1})
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unauthenticated, st.Code())
	require.Equal(t, ReasonLoginRequired, errorInfoOf(t, err).GetReason())
}

func TestE2E_NotFound(t *testing.T) {
	client, _ := startE2E(t, `{"code":404,"msg":"不存在"}`)

	_, err := client.GetSongDetail(context.Background(), &mmpb.GetSongDetailRequest{SongId: 1})
	st, _ := status.FromError(err)
	require.Equal(t, codes.NotFound, st.Code())
	require.Equal(t, ReasonResourceNotFound, errorInfoOf(t, err).GetReason())
}

// REST(grpc-gateway)侧:HTTP 状态码随 gRPC code 变化,响应体带 ErrorInfo,
// 非 gRPC 客户端(curl / 前端 / 其它语言的 HTTP 客户端)也能区分错误类型。
func TestE2E_Gateway_HTTPStatus(t *testing.T) {
	tests := []struct {
		name     string
		upstream string
		wantHTTP int
		reason   string
	}{
		{"未登录 → 401", `{"code":301}`, http.StatusUnauthorized, ReasonLoginRequired},
		{"不存在 → 404", `{"code":404}`, http.StatusNotFound, ReasonResourceNotFound},
		// 上游正常应答(code 200)但没有该资源:由 endpoint 的 MapResponse 判定,必须同样归为不存在,
		// 而不是落入 INTERNAL。
		{"空结果也是不存在 → 404", `{"code":200,"songs":[]}`, http.StatusNotFound, ReasonResourceNotFound},
		// 上游返回了无法解析的内容:UPSTREAM_INVALID_RESPONSE 必须真实可达。
		{"响应无法解析 → 500 + UPSTREAM_INVALID_RESPONSE", `<html>not json</html>`, http.StatusInternalServerError, ReasonUpstreamInvalidResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, mux := startE2E(t, tt.upstream)

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/songs/1", nil))
			require.Equal(t, tt.wantHTTP, rec.Code, rec.Body.String())

			var body struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Details []struct {
					Type   string `json:"@type"`
					Reason string `json:"reason"`
					Domain string `json:"domain"`
				} `json:"details"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.NotEmpty(t, body.Details, "响应体应带 details: %s", rec.Body.String())
			require.Equal(t, tt.reason, body.Details[0].Reason)
			require.Equal(t, ErrorDomain, body.Details[0].Domain)
		})
	}
}
