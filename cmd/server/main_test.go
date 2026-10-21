package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 一个监听上的慢请求不能耗尽另一个监听的宽限时间:同时关闭时,第二个监听拿到的 ctx 仍然有效。
func TestShutdownAllRunsConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	slow := func(ctx context.Context) error {
		<-ctx.Done() // 一直等到 deadline,像有请求迟迟不结束
		return ctx.Err()
	}
	var sawLiveCtx bool
	fast := func(ctx context.Context) error {
		sawLiveCtx = ctx.Err() == nil
		return nil
	}

	errs := shutdownAll(ctx, slow, fast)
	require.True(t, sawLiveCtx, "第二个监听开始关闭时 ctx 不应已过期")
	require.True(t, errors.Is(errs[0], context.DeadlineExceeded))
	require.NoError(t, errs[1])
}
