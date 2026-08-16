// Package kit
// proxy.go 实现代理 URL 的解析与校验(PRD-0018)。
//
// flag(--proxy)与 config(proxy key)共用本函数:单一校验真相。
// 空串 = 未设置(返回 nil,nil,回落环境变量层——那是 Go 默认 transport 的
// ProxyFromEnvironment 行为,本包不做任何注入)。
package kit

import (
	"fmt"
	"net/url"
)

// ProxySource 常量:UseProxy 注入时的来源标记,doctor 据此展示解析链。
const (
	ProxySourceFlag   = "flag"   // --proxy 显式覆盖
	ProxySourceConfig = "config" // config.toml 的 proxy key
)

// proxySchemes 是代理协议白名单——全部是标准库 http.Transport 原生支持,
// 零第三方依赖(http/https 为经典 HTTP 代理,socks5 由 net/http 自 Go 1.9 直连支持)。
var proxySchemes = map[string]bool{
	"http":   true,
	"https":  true,
	"socks5": true,
}

// ParseProxyURL 校验并解析代理地址。
//   - 空串 → (nil, nil):未设置,不注入任何代理(环境变量层自然生效)。
//   - scheme 必须显式写(http:// / https:// / socks5://):裸 host:port 报错并
//     引导补前缀——猜测默认值是文档陷阱。
//   - host 非空;端口可缺省(用 scheme 默认端口)。
func ParseProxyURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("代理地址 %q 无法解析(缺 scheme?应形如 http://127.0.0.1:7890): %w", raw, err)
	}
	if !proxySchemes[u.Scheme] {
		return nil, fmt.Errorf("代理 %q 的协议 %q 不在白名单(支持 http/https/socks5,注意显式写 scheme)", raw, u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("代理 %q 缺少 host(应形如 http://127.0.0.1:7890)", raw)
	}
	return u, nil
}
