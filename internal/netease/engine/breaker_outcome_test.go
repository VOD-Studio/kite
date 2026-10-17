package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	merrors "github.com/VOD-Studio/kite/errors"
)

// TestRecordResult 熔断器只为「上游不健康」计失败;上游正常应答的业务错误按成功处理;
// 调用方取消与本地错误从未到达上游,忽略。
func TestRecordResult(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name string
		ctx  context.Context
		err  error
		// want 是记录一次后相对「已有 1 次失败」的状态:
		// "failure" = 失败数 +1;"reset" = 失败数清零;"ignore" = 不变。
		want string
	}{
		{"nil 成功", context.Background(), nil, "reset"},
		{"上游不可用", context.Background(), fmt.Errorf("%w: HTTP 503", merrors.ErrUpstreamUnavailable), "failure"},
		{"被限流", context.Background(), fmt.Errorf("%w: code=460", merrors.ErrRateLimited), "failure"},
		{"未登录是业务错误", context.Background(), fmt.Errorf("%w: code=301", merrors.ErrUnauthorized), "reset"},
		{"资源不存在是业务错误", context.Background(), fmt.Errorf("%w: code=404", merrors.ErrNotFound), "reset"},
		{"响应格式无效是业务错误", context.Background(), merrors.ErrInvalidResponse, "reset"},
		{"本地序列化失败不计", context.Background(), errors.New("序列化参数失败"), "ignore"},
		// transport 用 %v 包装底层错误,errors.Is(context.Canceled) 失效,必须靠 ctx.Err() 判断。
		{"调用方取消(错误被包成上游不可用)", cancelled, fmt.Errorf("%w: context canceled", merrors.ErrUpstreamUnavailable), "ignore"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cb := newCircuitBreaker(10, time.Minute)
			cb.recordFailure() // 预置 1 次失败作为基线
			cb.recordResult(tc.ctx, tc.err)

			cb.mu.Lock()
			got := cb.failures
			cb.mu.Unlock()
			switch tc.want {
			case "failure":
				if got != 2 {
					t.Errorf("failures = %d, 期望 2(计一次失败)", got)
				}
			case "reset":
				if got != 0 {
					t.Errorf("failures = %d, 期望 0(清零)", got)
				}
			case "ignore":
				if got != 1 {
					t.Errorf("failures = %d, 期望保持 1(不计)", got)
				}
			}
		})
	}
}

// 上游持续不可用时仍应熔断(分类没有让熔断器失效)。
func TestRecordResult_UpstreamFailuresStillOpen(t *testing.T) {
	cb := newCircuitBreaker(3, time.Minute)
	for i := 0; i < 3; i++ {
		cb.recordResult(context.Background(), fmt.Errorf("%w: HTTP 503", merrors.ErrUpstreamUnavailable))
	}
	if cb.State() != stateOpen {
		t.Fatal("连续 3 次上游不可用应打开熔断器")
	}
}
