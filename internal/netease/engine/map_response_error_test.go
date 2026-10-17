package engine_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	merrors "github.com/VOD-Studio/kite/errors"
	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// MapResponse 失败的分类规则:
//   - 已带语义哨兵(不存在 / 未登录 / 限流 / 上游不可用)的错误原样保留类别,
//     不能被误报为「上游响应格式无效」;
//   - 没有明确类别的(JSON 解码失败、缺字段)说明上游响应形状不符预期,归为 ErrInvalidResponse。
//
// 否则 ErrInvalidResponse 在生产代码里没有任何产生点,对外承诺的
// UPSTREAM_INVALID_RESPONSE 永远不可达。
func TestMapResponseError_Classification(t *testing.T) {
	t.Parallel()

	srv, _ := mockNeteaseServer(t, `{"code":200,"songs":[{"id":1,"name":"歌"}]}`)
	eng := engine.New(engine.WithBaseURL(srv.URL))

	cases := []struct {
		name        string
		mapErr      error
		wantIs      error
		wantNotIs   error
		wantMessage string
	}{
		{"解码失败 → 响应无效", errors.New("解析歌曲详情失败: invalid character"), merrors.ErrInvalidResponse, nil, "解析歌曲详情失败"},
		{"不存在保持 NotFound", fmt.Errorf("歌曲不存在: %w", merrors.ErrNotFound), merrors.ErrNotFound, merrors.ErrInvalidResponse, "歌曲不存在"},
		{"未登录保持 Unauthorized", fmt.Errorf("未登录: %w", merrors.ErrUnauthorized), merrors.ErrUnauthorized, merrors.ErrInvalidResponse, "未登录"},
		{"限流保持 RateLimited", fmt.Errorf("%w: code=460", merrors.ErrRateLimited), merrors.ErrRateLimited, merrors.ErrInvalidResponse, "460"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ep := makeSongDetailEndpoint()
			ep.Cache = nil
			ep.MapResponse = func(*mmpb.GetSongDetailRequest, json.RawMessage) (*mmpb.GetSongDetailResponse, error) {
				return nil, tc.mapErr
			}
			req := &mmpb.GetSongDetailRequest{SongId: 1}

			for name, call := range map[string]func() error{
				"Execute": func() error {
					_, err := engine.Execute(eng, context.Background(), ep, req)
					return err
				},
				"ExecuteWithCookie": func() error {
					_, err := engine.ExecuteWithCookie(eng, engine.WithCookie(context.Background(), "MUSIC_U=x"), ep, req)
					return err
				},
			} {
				err := call()
				require.ErrorIs(t, err, tc.wantIs, name)
				if tc.wantNotIs != nil {
					require.NotErrorIs(t, err, tc.wantNotIs, name)
				}
				require.ErrorContains(t, err, tc.wantMessage, "%s: 原始信息不应丢失", name)
			}
		})
	}
}
