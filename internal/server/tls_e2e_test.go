package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// selfSignedCert 生成回环自签证书(测试专用),返回证书与私钥文件路径。
func selfSignedCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	certOut := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyOut := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	require.NoError(t, os.WriteFile(certFile, certOut, 0o600))
	require.NoError(t, os.WriteFile(keyFile, keyOut, 0o600))
	return certFile, keyFile
}

// tlsHTTPClient 接受自签证书的 REST 客户端(测试证书不为系统信任链所含)。
func tlsHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // 测试自签证书
	}}
}

func tlsGet(t *testing.T, url string, header http.Header) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(bg(t), http.MethodGet, url, nil)
	require.NoError(t, err)
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := tlsHTTPClient(t).Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body
}

// TLS 启用后:两个端口都只接受 TLS,令牌与业务请求经密文传输,gateway 到 gRPC 的内部
// 连接同样走 TLS。
func TestNewApp_TLS(t *testing.T) {
	certFile, keyFile := selfSignedCert(t)
	hits := &atomic.Int32{}
	up := newUpstream(t, hits)

	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(up.URL))
	app, err := NewApp(Options{
		GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0",
		AuthTokens:  single(testToken),
		TLSCertFile: certFile, TLSKeyFile: keyFile,
	}, eng, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	})

	t.Run("REST 走 https", func(t *testing.T) {
		code, body := tlsGet(t, "https://"+app.HTTPAddr()+"/healthz", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		require.JSONEq(t, `{"status":"SERVING"}`, string(body))

		code, body = tlsGet(t, "https://"+app.HTTPAddr()+"/api/v1/songs/1",
			http.Header{"Authorization": {"Bearer " + testToken}})
		require.Equal(t, http.StatusUnauthorized, code, string(body))
		require.Equal(t, ReasonLoginRequired, reasonOf(t, body), "TLS 下 gateway 内部拨号 gRPC 仍工作")
		require.Positive(t, hits.Load())
	})

	t.Run("明文请求 REST 端口被拒", func(t *testing.T) {
		req, err := http.NewRequestWithContext(bg(t), http.MethodGet, "http://"+app.HTTPAddr()+"/healthz", nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		// Go 的 TLS 监听对明文 HTTP 返回 400 "Client sent an HTTP request to an HTTPS server",
		// 而不是成功应答:密文端口上的内容没有以明文泄漏。
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("gRPC 走 TLS", func(t *testing.T) {
		conn, err := grpc.NewClient(app.GRPCAddr(),
			grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true}))) //nolint:gosec // 测试自签证书
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		resp, err := healthpb.NewHealthClient(conn).Check(bg(t), &healthpb.HealthCheckRequest{})
		require.NoError(t, err)
		require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())
	})

	t.Run("明文 gRPC 拨号被拒", func(t *testing.T) {
		conn, err := grpc.NewClient(app.GRPCAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		_, err = healthpb.NewHealthClient(conn).Check(bg(t), &healthpb.HealthCheckRequest{})
		require.Error(t, err, "TLS 监听不得接受明文 gRPC")
	})
}

// TLS 配置错误必须拒绝启动:半配证书或坏文件时静默退回明文等于裸奔。
func TestNewApp_TLS_Misconfigured(t *testing.T) {
	certFile, keyFile := selfSignedCert(t)
	eng := engine.New(engine.WithCache(cache.Noop{}))

	for name, opts := range map[string]Options{
		"只配证书":    {TLSCertFile: certFile},
		"只配私钥":    {TLSKeyFile: keyFile},
		"证书文件不存在": {TLSCertFile: filepath.Join(t.TempDir(), "missing.pem"), TLSKeyFile: keyFile},
		"证书内容不是证书": {
			TLSCertFile: writeFileT(t, "bad.pem", "not a cert"),
			TLSKeyFile:  keyFile,
		},
	} {
		t.Run(name, func(t *testing.T) {
			app, err := NewApp(opts, eng, nil)
			require.Error(t, err)
			require.Nil(t, app)
		})
	}
}

// 多令牌 + 限流的端到端:额度按调用方隔离,健康检查不受影响,REST 与 gRPC 同一套规则。
func TestNewApp_RateLimitPerCaller(t *testing.T) {
	hits := &atomic.Int32{}
	up := newUpstream(t, hits)
	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(up.URL))
	app, err := NewApp(Options{
		GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0",
		AuthTokens: []AuthToken{
			{Name: "blog", Value: "t-blog"},
			{Name: "bot", Value: "t-bot"},
		},
		RatePerMinute: 2,
	}, eng, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = app.Shutdown(ctx)
	})

	url := "http://" + app.HTTPAddr() + "/api/v1/songs/1"
	auth := func(token string) http.Header {
		return http.Header{"Authorization": {"Bearer " + token}}
	}

	// blog 打满额度:前两次到达业务层(上游 301 → LOGIN_REQUIRED),第三次被本服务限流。
	for i := 0; i < 2; i++ {
		code, body := httpGet(t, url, auth("t-blog"))
		require.Equal(t, http.StatusUnauthorized, code, string(body))
		require.Equal(t, ReasonLoginRequired, reasonOf(t, body))
	}
	code, body := httpGet(t, url, auth("t-blog"))
	require.Equal(t, http.StatusTooManyRequests, code, string(body))
	require.Equal(t, ReasonServerRateLimited, reasonOf(t, body))

	// bot 的额度独立,不受 blog 影响。
	code, body = httpGet(t, url, auth("t-bot"))
	require.Equal(t, http.StatusUnauthorized, code, string(body))
	require.Equal(t, ReasonLoginRequired, reasonOf(t, body))

	// 被限流的调用方不影响健康检查。
	for i := 0; i < 3; i++ {
		code, body := httpGet(t, "http://"+app.HTTPAddr()+"/healthz", nil)
		require.Equal(t, http.StatusOK, code, string(body))
	}
	require.Positive(t, hits.Load())
}

func newUpstream(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":301}`))
	}))
	t.Cleanup(up.Close)
	return up
}

func writeFileT(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}
