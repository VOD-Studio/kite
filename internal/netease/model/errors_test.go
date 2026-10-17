package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	merrors "github.com/VOD-Studio/kite/errors"
)

// 「语义上的失败」必须携带分类哨兵:对外(gRPC / REST)据此映射为稳定的 reason,
// 裸 fmt.Errorf 会被当成内部错误。
func TestDecode_SemanticErrorsCarrySentinel(t *testing.T) {
	t.Parallel()

	t.Run("用户不存在 → ErrNotFound", func(t *testing.T) {
		t.Parallel()
		_, err := DecodeUserDetail(json.RawMessage(`{"code":200,"profile":{"userId":0}}`))
		require.ErrorIs(t, err, merrors.ErrNotFound)
	})

	t.Run("未登录 → ErrUnauthorized", func(t *testing.T) {
		t.Parallel()
		_, err := DecodeLoginResponse(json.RawMessage(`{"code":200,"account":{"id":0}}`))
		require.ErrorIs(t, err, merrors.ErrUnauthorized)
	})
}
