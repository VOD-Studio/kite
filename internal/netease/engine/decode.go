// Package engine 的响应解码辅助。
//
// 各 endpoint 的 MapResponse 用这两个函数统一「JSON 解码 + 错误归类」,
// 不再各自手写 json.Unmarshal + 错误包装:
//
//   - Decode:严格解码。失败说明上游响应形状与解码器假设不符,错误带
//     ErrInvalidResponse 哨兵(gRPC 映射为 UPSTREAM_INVALID_RESPONSE,
//     客户端据此知道是上游接口可能变了)。
//   - DecodeLoose:宽松解码。失败按零值,不报错(写操作响应缺字段按零值)。
//
// 语义失败(资源不存在、未登录、限流)由各解码逻辑自带哨兵错误,与这里的
// 形状归类正交;callEndpoint 的 wrapMapResponseError 保证两类错误的原类别
// 都不被抹掉。
package engine

import (
	"encoding/json"
	"fmt"

	merrors "github.com/VOD-Studio/kite/errors"
)

// Decode 把原始响应 JSON 严格解码到 T;失败返回带 ErrInvalidResponse 哨兵的错误,
// what 进错误消息(如 "喜欢列表")。
func Decode[T any](raw json.RawMessage, what string) (T, error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, fmt.Errorf("%w: 解析%s失败: %w", merrors.ErrInvalidResponse, what, err)
	}
	return v, nil
}

// DecodeLoose 把原始响应 JSON 宽松解码到 T;失败返回零值,不报错。
func DecodeLoose[T any](raw json.RawMessage) T {
	var v T
	_ = json.Unmarshal(raw, &v) //nolint:errcheck // 宽松解码:缺字段/形状不符按零值
	return v
}
