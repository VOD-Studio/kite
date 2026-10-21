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

// AuthToken 是一条服务访问令牌:Name 标识调用方(限流与审计的单位),Token 是令牌本体。
// 单令牌部署(兼容旧配置)Name 为空。
type AuthToken struct {
	Name  string
	Token string
}

// ServerConfig 是 gRPC + gateway 服务相关配置。
type ServerConfig struct {
	// GRPCAddr 是 gRPC 监听地址(host:port)。默认只绑本机回环,见 docs/server-security.md。
	GRPCAddr string

	// HTTPAddr 是 grpc-gateway REST 监听地址(host:port)。默认只绑本机回环。
	HTTPAddr string

	// AuthToken 是调用方访问本服务必须携带的 bearer token(与上游网易云 cookie 无关)。
	// 与 AuthTokenFile / AuthTokensFile 互斥;都不为空表示不启用鉴权,此时只允许绑回环地址。
	AuthToken string

	// AuthTokenFile 是存放 token 的文件路径(Docker secret / k8s secret 的惯用方式),
	// 内容首尾空白会被去掉。
	AuthTokenFile string

	// AuthTokensFile 是多令牌文件路径:每行「名称:令牌」,空行与 # 开头的行忽略,
	// 用于按调用方发放令牌(限流 / 审计 / 吊销的单位,见 docs/server-security.md)。
	// 与 AuthToken / AuthTokenFile 互斥。
	AuthTokensFile string

	// RatePerMinute 是每个调用方每分钟允许的请求数,0 表示不限流。
	RatePerMinute int

	// TLSCertFile / TLSKeyFile 是 TLS 证书与私钥文件,必须同时设置;
	// 设置后 gRPC 与 REST 监听都走 TLS。
	TLSCertFile string
	TLSKeyFile  string

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

// ResolveAuthTokens 返回最终生效的全部访问令牌。
// 三个来源(AuthToken / AuthTokenFile / AuthTokensFile)同时设置超过一个视为配置歧义,返回错误;
// 都不设返回 nil(不启用鉴权)。单令牌来源返回一个 Name 为空的条目,行为与旧版一致。
func (s ServerConfig) ResolveAuthTokens() ([]AuthToken, error) {
	sources := 0
	if s.AuthToken != "" {
		sources++
	}
	if s.AuthTokenFile != "" {
		sources++
	}
	if s.AuthTokensFile != "" {
		sources++
	}
	if sources > 1 {
		return nil, errors.New("KITE_SERVER_AUTH_TOKEN / KITE_SERVER_AUTH_TOKEN_FILE / KITE_SERVER_AUTH_TOKENS_FILE 只能设置一个")
	}
	switch {
	case s.AuthToken != "":
		return []AuthToken{{Token: s.AuthToken}}, nil
	case s.AuthTokenFile != "":
		b, err := os.ReadFile(s.AuthTokenFile)
		if err != nil {
			return nil, fmt.Errorf("读取 token 文件: %w", err)
		}
		token := strings.TrimSpace(string(b))
		if token == "" {
			// 文件存在但为空:静默当作「未配置」会让服务在以为有鉴权时裸奔。
			return nil, fmt.Errorf("token 文件 %s 为空", s.AuthTokenFile)
		}
		return []AuthToken{{Token: token}}, nil
	case s.AuthTokensFile != "":
		return parseAuthTokensFile(s.AuthTokensFile)
	default:
		return nil, nil
	}
}

// parseAuthTokensFile 解析多令牌文件:每行「名称:令牌」,以第一个冒号分隔(令牌本身可以含冒号),
// 空行与 # 开头的行忽略。名称为空、重名或重值(同一令牌发给两个调用方)都会让限流与吊销产生
// 歧义,直接拒绝启动。
func parseAuthTokensFile(path string) ([]AuthToken, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 tokens 文件: %w", err)
	}
	var tokens []AuthToken
	names := map[string]bool{}
	values := map[string]bool{}
	for lineNo, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, token, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("tokens 文件 %s 第 %d 行缺少「名称:令牌」分隔符", path, lineNo+1)
		}
		name = strings.TrimSpace(name)
		token = strings.TrimSpace(token)
		if name == "" {
			return nil, fmt.Errorf("tokens 文件 %s 第 %d 行名称为空", path, lineNo+1)
		}
		if token == "" {
			return nil, fmt.Errorf("tokens 文件 %s 第 %d 行令牌为空", path, lineNo+1)
		}
		if names[name] {
			return nil, fmt.Errorf("tokens 文件 %s 第 %d 行名称 %q 重复", path, lineNo+1, name)
		}
		if values[token] {
			return nil, fmt.Errorf("tokens 文件 %s 第 %d 行令牌与其它调用方重复", path, lineNo+1)
		}
		names[name] = true
		values[token] = true
		tokens = append(tokens, AuthToken{Name: name, Token: token})
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("tokens 文件 %s 为空", path)
	}
	return tokens, nil
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
	cfg.Server.AuthTokensFile = envStr("KITE_SERVER_AUTH_TOKENS_FILE", cfg.Server.AuthTokensFile)
	cfg.Server.RatePerMinute = envInt("KITE_SERVER_RATE_PER_MINUTE", cfg.Server.RatePerMinute)
	cfg.Server.TLSCertFile = envStr("KITE_SERVER_TLS_CERT", cfg.Server.TLSCertFile)
	cfg.Server.TLSKeyFile = envStr("KITE_SERVER_TLS_KEY", cfg.Server.TLSKeyFile)
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
