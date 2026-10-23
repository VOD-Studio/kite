package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
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

	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/server"
)

// selfSignedCert 生成含回环 SAN 的自签证书(探针测试用),返回证书与私钥文件路径。
func selfSignedCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "kite"},
		DNSNames:     []string{"kite", "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true, // 同一张证书兼任探针侧的信任根(KITE_PROBE_CACERT 路径)
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	certFile = filepath.Join(t.TempDir(), "cert.pem")
	keyFile = filepath.Join(t.TempDir(), "key.pem")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
	return certFile, keyFile
}

// TLS 探测模式下明文默认地址自动升级;已是 https 的原样保留。
func TestHTTPSURL(t *testing.T) {
	require.Equal(t, "https://kite:3721", httpsURL("http://kite:3721"))
	require.Equal(t, "https://kite:3721", httpsURL("https://kite:3721"))
}

// 部署启用 KITE_SERVER_TLS_* 后,探针凭 KITE_PROBE_CACERT 走 TLS 完成同一套契约验证。
func TestProbeTLS(t *testing.T) {
	certFile, keyFile := selfSignedCert(t)

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(upstream.Close)
	eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(upstream.URL))
	app, err := server.NewApp(server.Options{
		GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0",
		AuthTokens:  []server.AuthToken{{Value: "correct"}},
		TLSCertFile: certFile, TLSKeyFile: keyFile,
	}, eng, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, app.Shutdown(ctx))
	})

	t.Run("CACERT 指向签发证书 → 契约验证贯通", func(t *testing.T) {
		t.Setenv("KITE_PROBE_CACERT", certFile)
		tlsConf, err := probeTLSConfig()
		require.NoError(t, err)
		require.NotNil(t, tlsConf)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// 传明文 URL 模拟部署者只配了 CACERT:probe 侧(run 函数)负责升级 https,
		// 这里直接验证 https 与明文两种写法下契约都贯通。
		require.NoError(t, probe(ctx, "https://"+app.HTTPAddr(), app.GRPCAddr(), "correct", tlsConf))
		require.Zero(t, upstreamCalls.Load())
	})

	t.Run("CACERT 不信任服务端证书 → 探测失败而不是放行", func(t *testing.T) {
		wrongCA, _ := selfSignedCert(t)
		t.Setenv("KITE_PROBE_CACERT", wrongCA)
		tlsConf, err := probeTLSConfig()
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = probe(ctx, "https://"+app.HTTPAddr(), app.GRPCAddr(), "correct", tlsConf)
		require.Error(t, err)
	})

	t.Run("未配置 TLS 的探针拨明文端口 → 失败", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.Error(t, probe(ctx, "http://"+app.HTTPAddr(), app.GRPCAddr(), "correct", nil))
	})
}

func TestProbeAuthenticationContract(t *testing.T) {
	for _, tc := range []struct {
		name        string
		serverToken string
		probeToken  string
		wantError   string
	}{
		{"有效令牌贯通两种协议且不访问上游", "correct", "correct", ""},
		{"错误令牌不能把健康端点当成部署成功", "correct", "wrong", "LOGIN_REQUIRED"},
		{"意外关闭鉴权必须拒绝发布", "", "correct", "SERVER_AUTH_FAILED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				upstreamCalls.Add(1)
				w.WriteHeader(http.StatusBadGateway)
			}))
			t.Cleanup(upstream.Close)
			eng := engine.New(engine.WithCache(cache.Noop{}), engine.WithBaseURL(upstream.URL))
			app, err := server.NewApp(server.Options{
				GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0",
				AuthTokens: []server.AuthToken{{Value: tc.serverToken}},
			}, eng, nil)
			require.NoError(t, err)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				require.NoError(t, app.Shutdown(ctx))
			})

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = probe(ctx, "http://"+app.HTTPAddr(), app.GRPCAddr(), tc.probeToken, nil)
			if tc.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantError)
			}
			require.Zero(t, upstreamCalls.Load(), "部署检查不得依赖网易云是否在线")
		})
	}
}
