// Package config 定义 kite 服务的配置结构。
//
// 配置分模块：Server / Provider / Redis / Worker 各自独立文件。
// 通过环境变量加载，KITE_ 前缀 + 模块名 + 字段名。
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config 是 kite 服务的顶层配置。
type Config struct {
	// Server 是 HTTP 服务相关配置。
	Server ServerConfig

	// Provider 是网易云 provider 相关配置。
	Provider ProviderConfig

	// Redis 是 Redis 连接配置（cache / store / asynq 共用）。
	Redis RedisConfig

	// Worker 是 Asynq worker 相关配置。
	Worker WorkerConfig

	// OTel 是 OpenTelemetry 追踪配置。
	OTel OTelConfig

	// Public 是给网页播放器用的公开只读模式配置,见 docs/player.md。
	Public PublicConfig
}

// PublicConfig 是公开只读模式(/player/v1)的配置。Addr 为空表示不开启。
//
// 公开模式不要求访问令牌,所以只暴露白名单里的歌单与歌曲,播放链接带签名并会过期;
// 各项的解析与校验在 internal/public.ParseOptions,配置错误时拒绝启动。
type PublicConfig struct {
	// Addr 是公开模式的监听地址(host:port),空表示不开启。
	Addr string

	// Playlists 是允许公开的歌单 ID,逗号分隔。
	Playlists string

	// Songs 是允许单独公开的歌曲 ID,逗号分隔(歌单里的歌无需重复列出)。
	Songs string

	// Origins 是允许跨域调用的站点(如 https://blog.example.com),逗号分隔;* 表示不限。
	Origins string

	// SessionFile 是服务账号的网易云登录态文件(server public-login 生成),空表示只按匿名播放。
	SessionFile string

	// SigningKeyFile 是播放链接签名密钥文件,空表示每次启动随机生成(重启后旧链接失效)。
	SigningKeyFile string

	// RatePerMinute 是每个来源 IP 每分钟允许的请求数。
	RatePerMinute int

	// TrustProxy 为 true 时按 X-Forwarded-For 取来源 IP(部署在反向代理之后才打开)。
	TrustProxy bool

	// Quality 是播放音质上限:standard / exhigh / lossless / hires。
	Quality string
}

// ServerConfig 是 gRPC + gateway 服务相关配置。
type ServerConfig struct {
	// GRPCAddr 是 gRPC 监听地址(host:port)。默认只绑本机回环,见 docs/server-security.md。
	GRPCAddr string

	// HTTPAddr 是 grpc-gateway REST 监听地址(host:port)。默认只绑本机回环。
	HTTPAddr string

	// AuthToken 是调用方访问本服务必须携带的 bearer token(与上游网易云 cookie 无关)。
	// 与 AuthTokenFile 互斥;都为空表示不启用鉴权,此时只允许绑回环地址。
	AuthToken string

	// AuthTokenFile 是存放 token 的文件路径(Docker secret / k8s secret 的惯用方式),
	// 内容首尾空白会被去掉。
	AuthTokenFile string

	// Env 是运行环境（dev / prod）。
	//
	// dev 环境用彩色文本日志，prod 环境用 JSON 日志。
	Env string
}

// ProviderConfig 是网易云 provider 相关配置。
type ProviderConfig struct {
	// UpstreamTimeout 是调用网易云上游的超时时间，单位秒。
	UpstreamTimeout int

	// MaxRetries 是调用上游失败后的最大重试次数。
	MaxRetries int
}

// RedisConfig 是 Redis 连接配置。
//
// cache / store / asynq 三个组件共用同一个 Redis 连接。
type RedisConfig struct {
	// Host 是 Redis 主机地址。
	Host string

	// Port 是 Redis 端口。
	Port int

	// Password 是 Redis 密码，无密码时为空。
	Password string

	// DB 是 Redis 数据库编号。
	DB int

	// PoolSize 是 Redis 连接池大小（cache / store / asynq 共用）。
	PoolSize int
}

// WorkerConfig 是 Asynq worker 相关配置。
type WorkerConfig struct {
	// Concurrency 是 worker 并发处理的任务数。
	Concurrency int

	// CookieCheckInterval 是 Cookie 健康检查的轮询间隔，单位小时。
	CookieCheckInterval int
}

// OTelConfig 是 OpenTelemetry 追踪配置。
type OTelConfig struct {
	// Exporter 是 exporter 类型：none（noop，本地开发默认）/ otlp-grpc / otlp-http。
	Exporter string

	// Endpoint 是 OTLP collector 地址（如 localhost:4317）。
	// 仅 Exporter 非 none 时生效。
	Endpoint string

	// ServiceName 是上报到后端的服务名（如 kite）。
	ServiceName string

	// SampleRatio 是根 span 采样率，0.0-1.0，1.0 表示全采样。
	SampleRatio float64
}

// Default 返回默认配置。
func Default() Config {
	return Config{
		Server: ServerConfig{
			GRPCAddr: "127.0.0.1:3722",
			HTTPAddr: "127.0.0.1:3721",
			Env:      "dev",
		},
		Provider: ProviderConfig{
			UpstreamTimeout: 10,
			MaxRetries:      3,
		},
		Redis: RedisConfig{
			Host:     "localhost",
			Port:     6379,
			DB:       1, // 用 DB 1 避免和 mimo-blog（DB 0）冲突
			PoolSize: 10,
		},
		Worker: WorkerConfig{
			Concurrency:         5,
			CookieCheckInterval: 6,
		},
		OTel: OTelConfig{
			Exporter:    "none",
			ServiceName: "kite",
			SampleRatio: 1.0,
		},
		Public: PublicConfig{
			RatePerMinute: 120,
			Quality:       "exhigh",
		},
	}
}

// ResolveAuthToken 返回最终生效的访问令牌:AuthToken 或 AuthTokenFile 的内容(去掉首尾空白)。
// 两者同时设置视为配置歧义,返回错误;都不设返回空串(不启用鉴权)。
func (s ServerConfig) ResolveAuthToken() (string, error) {
	if s.AuthToken != "" && s.AuthTokenFile != "" {
		return "", errors.New("KITE_SERVER_AUTH_TOKEN 与 KITE_SERVER_AUTH_TOKEN_FILE 只能设置一个")
	}
	if s.AuthTokenFile == "" {
		return s.AuthToken, nil
	}
	b, err := os.ReadFile(s.AuthTokenFile)
	if err != nil {
		return "", fmt.Errorf("读取 token 文件: %w", err)
	}
	token := strings.TrimSpace(string(b))
	if token == "" {
		// 文件存在但为空:静默当作「未配置」会让服务在以为有鉴权时裸奔。
		return "", fmt.Errorf("token 文件 %s 为空", s.AuthTokenFile)
	}
	return token, nil
}

// Addr 返回 Redis 地址（host:port 格式）。
func (r RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

// Load 从环境变量加载配置，覆盖默认值。
//
// 环境变量前缀为 KITE_，格式：KITE_{MODULE}_{FIELD}。
// 例如 KITE_SERVER_PORT、KITE_REDIS_HOST。
func Load() Config {
	cfg := Default()

	cfg.Server.GRPCAddr = envStr("KITE_SERVER_GRPC_ADDR", cfg.Server.GRPCAddr)
	cfg.Server.HTTPAddr = envStr("KITE_SERVER_HTTP_ADDR", cfg.Server.HTTPAddr)
	cfg.Server.AuthToken = envStr("KITE_SERVER_AUTH_TOKEN", cfg.Server.AuthToken)
	cfg.Server.AuthTokenFile = envStr("KITE_SERVER_AUTH_TOKEN_FILE", cfg.Server.AuthTokenFile)
	cfg.Server.Env = envStr("KITE_SERVER_ENV", cfg.Server.Env)
	cfg.Provider.UpstreamTimeout = envInt("KITE_PROVIDER_UPSTREAM_TIMEOUT", cfg.Provider.UpstreamTimeout)
	cfg.Provider.MaxRetries = envInt("KITE_PROVIDER_MAX_RETRIES", cfg.Provider.MaxRetries)
	cfg.Redis.Host = envStr("KITE_REDIS_HOST", cfg.Redis.Host)
	cfg.Redis.Port = envInt("KITE_REDIS_PORT", cfg.Redis.Port)
	cfg.Redis.Password = envStr("KITE_REDIS_PASSWORD", cfg.Redis.Password)
	cfg.Redis.DB = envInt("KITE_REDIS_DB", cfg.Redis.DB)
	cfg.Redis.PoolSize = envInt("KITE_REDIS_POOL_SIZE", cfg.Redis.PoolSize)
	cfg.Worker.Concurrency = envInt("KITE_WORKER_CONCURRENCY", cfg.Worker.Concurrency)
	cfg.Worker.CookieCheckInterval = envInt("KITE_WORKER_COOKIE_CHECK_INTERVAL", cfg.Worker.CookieCheckInterval)
	cfg.OTel.Exporter = envStr("KITE_OTEL_EXPORTER", cfg.OTel.Exporter)
	cfg.OTel.Endpoint = envStr("KITE_OTEL_ENDPOINT", cfg.OTel.Endpoint)
	cfg.OTel.ServiceName = envStr("KITE_OTEL_SERVICE_NAME", cfg.OTel.ServiceName)
	cfg.OTel.SampleRatio = envFloat("KITE_OTEL_SAMPLE_RATIO", cfg.OTel.SampleRatio)
	cfg.Public.Addr = envStr("KITE_PUBLIC_ADDR", cfg.Public.Addr)
	cfg.Public.Playlists = envStr("KITE_PUBLIC_PLAYLISTS", cfg.Public.Playlists)
	cfg.Public.Songs = envStr("KITE_PUBLIC_SONGS", cfg.Public.Songs)
	cfg.Public.Origins = envStr("KITE_PUBLIC_ORIGINS", cfg.Public.Origins)
	cfg.Public.SessionFile = envStr("KITE_PUBLIC_SESSION_FILE", cfg.Public.SessionFile)
	cfg.Public.SigningKeyFile = envStr("KITE_PUBLIC_SIGNING_KEY_FILE", cfg.Public.SigningKeyFile)
	cfg.Public.RatePerMinute = envInt("KITE_PUBLIC_RATE_PER_MINUTE", cfg.Public.RatePerMinute)
	cfg.Public.TrustProxy = envStr("KITE_PUBLIC_TRUST_PROXY", "") == "true"
	cfg.Public.Quality = envStr("KITE_PUBLIC_QUALITY", cfg.Public.Quality)

	cfg.Server.Env = strings.ToLower(cfg.Server.Env)
	return cfg
}

// String 返回配置摘要，用于启动日志。
func (c Config) String() string {
	return fmt.Sprintf("env=%s grpc=%s http=%s redis=%s db=%d otel=%s", c.Server.Env, c.Server.GRPCAddr, c.Server.HTTPAddr, c.Redis.Addr(), c.Redis.DB, c.OTel.Exporter)
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
