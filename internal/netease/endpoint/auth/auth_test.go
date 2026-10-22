package auth

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// MapResponse 直测:解出的字段、宽容分支(不报错)与严格分支(报错)逐格钉死。
func TestLoginCellphone_MapResponse(t *testing.T) {
	t.Parallel()

	// 正常:解出账号信息。
	resp, err := LoginCellphone.MapResponse(nil, json.RawMessage(
		`{"code":200,"account":{"id":42},"profile":{"nickname":"风筝"}}`))
	require.NoError(t, err)
	require.Equal(t, int64(42), resp.GetSession().GetUserId())
	require.Equal(t, "风筝", resp.GetSession().GetNickname())

	// 上游 200 + 下发 Set-Cookie 但 account.id 解不出:宽容返回空 Session、不报错
	// ——CLI 据此落盘仅 cookie 的会话(与 origin/main 的 persistLogin 行为一致),
	// 严格性由 service 侧按「无账号」补回。
	resp, err = LoginCellphone.MapResponse(nil, json.RawMessage(`{"code":200}`))
	require.NoError(t, err)
	require.Nil(t, resp.GetSession(), "解不出账号不应带 Session,也不应报错")
}

func TestCheckQrcode_MapResponse(t *testing.T) {
	t.Parallel()

	// 803(已确认):响应体含账号信息,一并解出 Session。
	resp, err := CheckQrcode.MapResponse(nil, json.RawMessage(
		`{"code":803,"account":{"id":7},"profile":{"nickname":"七"}}`))
	require.NoError(t, err)
	require.Equal(t, mmpb.QrcodeCode_QRCODE_CODE_CONFIRMED, resp.GetCode())
	require.Equal(t, int64(7), resp.GetSession().GetUserId())

	// 803 但解不出账号:不带 Session、不报错(cookie 由 Set-Cookie 回传,调用方决定)。
	resp, err = CheckQrcode.MapResponse(nil, json.RawMessage(`{"code":803,"message":"ok"}`))
	require.NoError(t, err)
	require.Nil(t, resp.GetSession())

	// 等待扫描:只有 code/message。
	resp, err = CheckQrcode.MapResponse(nil, json.RawMessage(`{"code":801,"message":"等待扫码"}`))
	require.NoError(t, err)
	require.Equal(t, mmpb.QrcodeCode_QRCODE_CODE_WAITING, resp.GetCode())
	require.Nil(t, resp.GetSession())
}

func TestLoginStatus_MapResponse(t *testing.T) {
	t.Parallel()

	resp, err := LoginStatus.MapResponse(nil, json.RawMessage(
		`{"code":200,"account":{"id":9},"profile":{"nickname":"九"}}`))
	require.NoError(t, err)
	require.Equal(t, int64(9), resp.GetSession().GetUserId())

	// 未登录(无账号信息)不算错误,空 Session 即「未登录」语义。
	resp, err = LoginStatus.MapResponse(nil, json.RawMessage(`{"code":200}`))
	require.NoError(t, err)
	require.Nil(t, resp.GetSession())
}

func TestLoginQrcode_MapResponse(t *testing.T) {
	t.Parallel()

	resp, err := LoginQrcode.MapResponse(nil, json.RawMessage(`{"code":200,"unikey":"k-123"}`))
	require.NoError(t, err)
	require.Equal(t, "k-123", resp.GetQrcode().GetKey())
	require.Equal(t, QrcodeURL("k-123"), resp.GetQrcode().GetUrl())

	_, err = LoginQrcode.MapResponse(nil, json.RawMessage(`{"code":200}`))
	require.Error(t, err, "拿不到 unikey 应报错(经 callEndpoint 后归类为无效响应)")
}
