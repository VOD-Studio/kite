package engine

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	merrors "github.com/VOD-Studio/kite/errors"
)

type decodeResp struct {
	IDs []int64 `json:"ids"`
}

func TestDecode(t *testing.T) {
	t.Parallel()

	v, err := Decode[decodeResp](json.RawMessage(`{"ids":[1,2]}`), "喜欢列表")
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, v.IDs)

	// 形状不符:带 ErrInvalidResponse 哨兵,客户端可据此识别「上游响应变了」。
	_, err = Decode[decodeResp](json.RawMessage(`{"ids":`), "喜欢列表")
	require.ErrorIs(t, err, merrors.ErrInvalidResponse)
	require.ErrorContains(t, err, "喜欢列表")

	// wrapMapResponseError 保留已带哨兵错误的类别,不再二次归类。
	wrapped := wrapMapResponseError(err)
	require.ErrorIs(t, wrapped, merrors.ErrInvalidResponse)
}

func TestDecodeLoose(t *testing.T) {
	t.Parallel()

	v := DecodeLoose[decodeResp](json.RawMessage(`{"ids":[3]}`))
	require.Equal(t, []int64{3}, v.IDs)

	// 形状不符按零值,不报错。
	v = DecodeLoose[decodeResp](json.RawMessage(`{"ids":`))
	require.Empty(t, v.IDs)
}
