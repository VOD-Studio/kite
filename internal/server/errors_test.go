package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	merrors "github.com/VOD-Studio/kite/errors"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
)

// errorInfoOf 取出 status 里的 ErrorInfo。
func errorInfoOf(t *testing.T, err error) *errdetails.ErrorInfo {
	t.Helper()
	st, ok := status.FromError(err)
	require.True(t, ok, "应为 gRPC status error: %v", err)
	for _, d := range st.Details() {
		if info, ok := d.(*errdetails.ErrorInfo); ok {
			return info
		}
	}
	t.Fatalf("status 里没有 ErrorInfo: %v", st)
	return nil
}

func TestToStatusError_Mapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		code   codes.Code
		reason string
	}{
		{"未登录", fmt.Errorf("%w: code=301", merrors.ErrUnauthorized), codes.Unauthenticated, ReasonLoginRequired},
		{"无可用 session", session.ErrNoAvailableSession, codes.Unauthenticated, ReasonLoginRequired},
		{"资源不存在", fmt.Errorf("%w: code=404", merrors.ErrNotFound), codes.NotFound, ReasonResourceNotFound},
		{"被限流", fmt.Errorf("%w: HTTP 460", merrors.ErrRateLimited), codes.ResourceExhausted, ReasonRateLimited},
		{"上游不可用", fmt.Errorf("%w: HTTP 503", merrors.ErrUpstreamUnavailable), codes.Unavailable, ReasonUpstreamUnavailable},
		{"熔断打开", engine.ErrCircuitOpen, codes.Unavailable, ReasonCircuitOpen},
		{"上游响应无效", merrors.ErrInvalidResponse, codes.Internal, ReasonUpstreamInvalidResponse},
		{"不支持", merrors.ErrUnsupportedPlatform, codes.Unimplemented, ReasonUnsupported},
		{"未分类", errors.New("boom"), codes.Internal, ReasonInternal},
		// 本地凭据缺失:engine 用 %w+%w 同时携带「未发出」标记与 ErrUnauthorized,映射仍应是未登录。
		{"多重包装仍可识别", fmt.Errorf("outer: %w", fmt.Errorf("%w: %w", errors.New("请求未发出"), merrors.ErrUnauthorized)),
			codes.Unauthenticated, ReasonLoginRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := toStatusError(context.Background(), "/x/Y", tt.err)
			st, ok := status.FromError(got)
			require.True(t, ok)
			require.Equal(t, tt.code, st.Code())
			info := errorInfoOf(t, got)
			require.Equal(t, tt.reason, info.GetReason())
			require.Equal(t, ErrorDomain, info.GetDomain())
			require.Equal(t, "netease", info.GetMetadata()["provider"])
		})
	}
}

func TestToStatusError_NilAndPassThrough(t *testing.T) {
	t.Parallel()

	require.NoError(t, toStatusError(context.Background(), "/x/Y", nil))

	orig := status.Error(codes.InvalidArgument, "bad arg")
	require.Equal(t, orig, toStatusError(context.Background(), "/x/Y", orig), "已是 status 的错误不应被覆盖")
}

// 请求 context 已结束时按 ctx 映射。engine 的 transport 用 %v 包装底层错误,
// 错误链里没有 context 错误,只能看 ctx.Err()。
func TestToStatusError_ContextDone(t *testing.T) {
	t.Parallel()

	upstream := fmt.Errorf("%w: context canceled", merrors.ErrUpstreamUnavailable)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	st, _ := status.FromError(toStatusError(canceled, "/x/Y", upstream))
	require.Equal(t, codes.Canceled, st.Code())

	expired, cancel2 := context.WithTimeout(context.Background(), 0)
	defer cancel2()
	<-expired.Done()
	st, _ = status.FromError(toStatusError(expired, "/x/Y", upstream))
	require.Equal(t, codes.DeadlineExceeded, st.Code())
}

// 未分类错误不向调用方泄露内部细节。
func TestToStatusError_InternalHidesDetails(t *testing.T) {
	t.Parallel()

	st, _ := status.FromError(toStatusError(context.Background(), "/x/Y", errors.New("dial tcp 10.0.0.5:6379: secret-host")))
	require.Equal(t, codes.Internal, st.Code())
	require.NotContains(t, st.Message(), "secret-host")
	require.NotContains(t, st.Message(), "10.0.0.5")
}

func TestErrorInterceptor(t *testing.T) {
	t.Parallel()

	info := &grpc.UnaryServerInfo{FullMethod: "/netease.music.v1.SongService/GetSongDetail"}

	resp, err := ErrorInterceptor(context.Background(), nil, info, func(context.Context, any) (any, error) { return "ok", nil })
	require.NoError(t, err)
	require.Equal(t, "ok", resp)

	_, err = ErrorInterceptor(context.Background(), nil, info, func(context.Context, any) (any, error) {
		return nil, merrors.ErrNotFound
	})
	st, _ := status.FromError(err)
	require.Equal(t, codes.NotFound, st.Code())
}

// docs/grpc-errors.md 的映射表是对外契约的文档,与 Reason* 常量是同一份信息:
// 新增 / 改名 reason 必须同步文档,漂移即红(AGENTS.md:不要靠记忆维护第二份真相)。
func TestReasonsDocumented(t *testing.T) {
	t.Parallel()

	doc, err := os.ReadFile("../../docs/grpc-errors.md")
	require.NoError(t, err)

	reasons := []string{
		ReasonLoginRequired, ReasonResourceNotFound, ReasonRateLimited, ReasonUpstreamUnavailable,
		ReasonCircuitOpen, ReasonUpstreamInvalidResponse, ReasonUnsupported, ReasonInternal,
		ReasonCanceled, ReasonDeadlineExceeded,
	}
	for _, r := range reasons {
		require.Contains(t, string(doc), "`"+r+"`", "reason %s 未出现在 docs/grpc-errors.md 的映射表里", r)
	}
	require.Contains(t, string(doc), "`"+ErrorDomain+"`", "ErrorDomain 未出现在文档里")
}
