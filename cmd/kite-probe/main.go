// kite-probe 是发布镜像内的运维工具,从调用方网络检查本地 REST/gRPC 契约,不访问网易云。
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "部署探针失败:", err)
		os.Exit(1)
	}
	fmt.Println("ok: REST/gRPC 健康、服务鉴权及 LOGIN_REQUIRED 契约")
}

func run() error {
	tokenFile := envOr("KITE_SERVER_AUTH_TOKEN_FILE", "/run/secrets/kite-auth-token")
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return fmt.Errorf("读取服务令牌: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return errors.New("服务令牌为空或含换行")
	}

	// 部署启用 KITE_SERVER_TLS_* 时,探针同样走 TLS:
	//   KITE_PROBE_CACERT  私有 CA 证书文件(隐含开启 TLS,REST 与 gRPC 都用它验证服务端证书)
	//   KITE_PROBE_TLS     证书由公共 CA 签发时置 true,用系统信任链验证
	// 两者都不设置时保持明文探测(生产部署链路现状)。
	tlsConf, err := probeTLSConfig()
	if err != nil {
		return err
	}
	httpURL := envOr("KITE_PROBE_HTTP_URL", "http://kite:3721")
	if tlsConf != nil {
		httpURL = httpsURL(httpURL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return probe(ctx, httpURL, envOr("KITE_PROBE_GRPC_ADDR", "kite:3722"), token, tlsConf)
}

// probeTLSConfig 按探测环境变量构造客户端 TLS 配置;两个变量都未设置返回 nil(明文探测)。
func probeTLSConfig() (*tls.Config, error) {
	caPath := envOr("KITE_PROBE_CACERT", "")
	if caPath == "" && !strings.EqualFold(envOr("KITE_PROBE_TLS", ""), "true") {
		return nil, nil
	}
	conf := &tls.Config{}
	if caPath == "" {
		return conf, nil // 公共 CA:沿用系统信任链(nil RootCAs)
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("读取 KITE_PROBE_CACERT: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("KITE_PROBE_CACERT %s 里没有可用的 PEM 证书", caPath)
	}
	conf.RootCAs = pool
	return conf, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// httpsURL 把明文探测地址升级为 https:TLS 探测模式下,默认地址与旧配置仍是 http:// 写法,
// 部署者只需配 KITE_PROBE_CACERT / KITE_PROBE_TLS 一个变量。
func httpsURL(url string) string { return strings.Replace(url, "http://", "https://", 1) }

// probe 对指定地址做部署验证。tlsConf 非 nil 时 REST 与 gRPC 都走 TLS 并按它验证服务端证书。
func probe(ctx context.Context, httpURL, grpcAddr, token string, tlsConf *tls.Config) error {
	// 内部服务名必须直接解析,不能因 runner/宿主机的代理环境而探测到另一个入口。
	transport := &http.Transport{Proxy: nil, TLSClientConfig: tlsConf}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	base := strings.TrimRight(httpURL, "/")
	var health struct {
		Status string `json:"status"`
	}
	code, err := getJSON(ctx, client, base+"/healthz", "", &health)
	if err != nil {
		return fmt.Errorf("REST 健康检查: %w", err)
	}
	if code != http.StatusOK || health.Status != "SERVING" {
		return fmt.Errorf("REST 健康状态异常(HTTP %d)", code)
	}
	for _, check := range []struct{ token, reason string }{
		{"", "SERVER_AUTH_FAILED"}, {token, "LOGIN_REQUIRED"},
	} {
		var body struct {
			Code    int `json:"code"`
			Details []struct {
				Type   string `json:"@type"`
				Domain string `json:"domain"`
				Reason string `json:"reason"`
			} `json:"details"`
		}
		code, err = getJSON(ctx, client, base+"/api/v1/recommend/daily", check.token, &body)
		if err != nil {
			return fmt.Errorf("REST %s 检查: %w", check.reason, err)
		}
		matched := false
		for _, detail := range body.Details {
			if detail.Type == "type.googleapis.com/google.rpc.ErrorInfo" && detail.Domain == "kite.music" && detail.Reason == check.reason {
				matched = true
				break
			}
		}
		if code != http.StatusUnauthorized || body.Code != int(codes.Unauthenticated) || !matched {
			return fmt.Errorf("REST 预期 %s,实际 HTTP %d/code %d", check.reason, code, body.Code)
		}
	}

	// 探针部署环境里服务端证书多为内部 CA 签发;tlsConf 携带 KITE_PROBE_CACERT 的信任根。
	creds := insecure.NewCredentials()
	if tlsConf != nil {
		creds = credentials.NewTLS(tlsConf)
	}
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(creds), grpc.WithNoProxy())
	if err != nil {
		return fmt.Errorf("创建 gRPC 客户端: %w", err)
	}
	defer func() { _ = conn.Close() }()
	result, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		return fmt.Errorf("gRPC 健康检查: %w", err)
	}
	if result.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return errors.New("gRPC 未就绪")
	}
	rpc := mmpb.NewRecommendServiceClient(conn)
	for _, check := range []struct{ token, reason string }{
		{"", "SERVER_AUTH_FAILED"}, {token, "LOGIN_REQUIRED"},
	} {
		requestCtx := ctx
		if check.token != "" {
			requestCtx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+check.token)
		}
		_, err = rpc.GetDailyRecommend(requestCtx, &mmpb.GetDailyRecommendRequest{})
		matched := false
		for _, detail := range status.Convert(err).Details() {
			if info, ok := detail.(*errdetails.ErrorInfo); ok && info.GetDomain() == "kite.music" && info.GetReason() == check.reason {
				matched = true
				break
			}
		}
		if status.Code(err) != codes.Unauthenticated || !matched {
			return fmt.Errorf("gRPC 预期 %s,实际 %s", check.reason, status.Code(err))
		}
	}
	return nil
}

func getJSON(ctx context.Context, client *http.Client, address, token string, body any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return 0, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(body); err != nil {
		return resp.StatusCode, fmt.Errorf("解析响应: %w", err)
	}
	return resp.StatusCode, nil
}
