package engine_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// 业务错误(未登录 / 不存在)说明上游正常应答,不应累计到熔断器上连坐全部接口。
func TestExecute_BusinessErrorsDoNotTripBreaker(t *testing.T) {
	t.Parallel()
	var failMode atomic.Bool
	failMode.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if failMode.Load() {
			_, _ = w.Write([]byte(`{"code":301,"msg":"需要登录"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":200,"songs":[{"id":1,"name":"歌"}]}`))
	}))
	t.Cleanup(srv.Close)

	eng := engine.New(engine.WithBaseURL(srv.URL))
	ep := makeSongDetailEndpoint()
	req := &mmpb.GetSongDetailRequest{SongId: 1}

	// 阈值是连续 5 次失败;发 8 次业务错误,若被计数熔断器早已打开。
	for i := 0; i < 8; i++ {
		_, err := engine.Execute(eng, context.Background(), ep, req)
		require.ErrorIs(t, err, merrors.ErrUnauthorized, "第 %d 次应是业务错误而不是熔断", i+1)
		require.NotErrorIs(t, err, engine.ErrCircuitOpen)
	}

	failMode.Store(false)
	resp, err := engine.Execute(eng, context.Background(), ep, req)
	require.NoError(t, err, "上游恢复后应立即可用")
	require.NotNil(t, resp)
}
