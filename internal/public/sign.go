package public

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"time"
)

var (
	errLinkExpired = errors.New("播放链接已过期")
	errLinkInvalid = errors.New("播放链接签名无效")
)

// signer 给播放链接签名:只有出现在白名单歌单 / 歌曲响应里的链接才能换到上游直链,
// 带了服务账号登录态时也不会变成「任意 ID 都能解析 VIP 链接」的开放代理。
type signer struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

// streamQuery 返回某首歌播放链接的查询串。过期时间按 10 分钟取整,
// 让同一时间段内的链接一致,浏览器与 CDN 缓存才有效。
func (s signer) streamQuery(id int64) string {
	exp := s.now().Add(s.ttl).Truncate(10 * time.Minute).Unix()
	e := strconv.FormatInt(exp, 10)
	return "exp=" + e + "&sig=" + s.mac(id, e)
}

func (s signer) verify(id int64, exp, sig string) error {
	ts, err := strconv.ParseInt(exp, 10, 64)
	if err != nil {
		return errLinkInvalid
	}
	if !hmac.Equal([]byte(sig), []byte(s.mac(id, exp))) {
		return errLinkInvalid
	}
	if s.now().Unix() > ts {
		return errLinkExpired
	}
	return nil
}

func (s signer) mac(id int64, exp string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte("stream:" + strconv.FormatInt(id, 10) + ":" + exp))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)[:16])
}
