// Package service 的 cookie override 执行辅助。
//
// 第三条执行路径（ADR §4.5）：写操作/特定登录态接口走 cookie override，
// cookie 由 interceptor 从 metadata 注入 context，绕过 SessionStore 轮换。
package service

import (
	"context"

	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// executeOverride 走 cookie override 路径执行 endpoint(cookie 从 context 取,不经 session 池)。
//
// cookie 由 interceptor 从 metadata "x-netease-cookie" 注入 context,对 service 透明。
// 执行语义在 engine.ExecuteWithCookie,本函数仅为 service 方法保留的短名。
func executeOverride[Req, Resp any](
	eng *engine.Engine,
	ctx context.Context,
	ep *engine.Endpoint[Req, Resp],
	req Req,
) (Resp, error) {
	return engine.ExecuteWithCookie(eng, ctx, ep, req)
}
