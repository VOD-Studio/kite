// Package main 是 kite 服务的入口。
//
// 启动 gRPC server（对外强类型 RPC）+ grpc-gateway（REST 暴露）双 server，
// 收到 SIGINT/SIGTERM 时优雅关闭。
//
// 核心引擎不接入自动选号池;可通过 KITE_SERVER_ACCOUNTS_FILE 托管显式选择的账号。
//
// 配置走环境变量(见 config.Load):默认只监听 127.0.0.1:3722(gRPC)与 127.0.0.1:3721(REST);
// 对外监听必须配置 KITE_SERVER_AUTH_TOKEN,否则拒绝启动。
//
// 设置 KITE_PUBLIC_ADDR 时另开公开只读模式(/player/v1,给网页播放器用,见 docs/player.md);
// 子命令 `server public-login` 扫码登录服务账号,通过 --account 选择 KITE_PUBLIC_ACCOUNTS 中的账号。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/VOD-Studio/kite/config"
	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/account"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/public"
	"github.com/VOD-Studio/kite/internal/server"
)

// startupCtx 是启动阶段没有请求 ctx 时用的兜底 context。
var startupCtx = context.Background()

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "public-login":
			os.Exit(runPublicLogin(os.Args[2:]))
		default:
			_, _ = fmt.Fprintf(os.Stderr, "未知子命令 %q。用法:server(启动服务)或 server public-login [--account ID | --file 路径]\n", os.Args[1])
			os.Exit(2)
		}
	}

	// 未指定账号的请求不会借用托管账号;公开播放器的服务账号另行配置。
	eng := engine.New(
		engine.WithCache(cache.Noop{}),
	)

	cfg := config.Load()
	var accounts *account.Store
	if cfg.Server.AccountsFile != "" {
		var err error
		accounts, err = account.Open(cfg.Server.AccountsFile)
		if err != nil {
			slog.ErrorContext(startupCtx, "load managed accounts failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}
	tokens, err := cfg.Server.ResolveAuthTokens()
	if err != nil {
		slog.ErrorContext(startupCtx, "load auth tokens failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	authTokens := make([]server.AuthToken, len(tokens))
	for i, t := range tokens {
		authTokens[i] = server.AuthToken{Name: t.Name, Value: t.Token}
	}

	app, err := server.NewApp(server.Options{
		GRPCAddr:      cfg.Server.GRPCAddr,
		HTTPAddr:      cfg.Server.HTTPAddr,
		AuthTokens:    authTokens,
		RatePerMinute: cfg.Server.RatePerMinute,
		TLSCertFile:   cfg.Server.TLSCertFile,
		TLSKeyFile:    cfg.Server.TLSKeyFile,
	}, eng, accounts)
	if err != nil {
		slog.ErrorContext(startupCtx, "init server failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	var pub *public.Server
	if cfg.Public.Addr != "" {
		opts, err := public.ParseOptions(cfg.Public)
		if err != nil {
			slog.ErrorContext(startupCtx, "public mode config invalid", slog.String("error", err.Error()))
			os.Exit(1)
		}
		pub = public.New(eng, opts)
		if err := pub.Start(); err != nil {
			slog.ErrorContext(startupCtx, "start public server failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}

	slog.InfoContext(startupCtx, "server ready",
		slog.String("grpc", app.GRPCAddr()), slog.String("http", app.HTTPAddr()),
		slog.Bool("auth", len(authTokens) > 0), slog.Int("callers", len(authTokens)),
		slog.Bool("tls", cfg.Server.TLSCertFile != ""), slog.Int("rate_per_minute", cfg.Server.RatePerMinute))

	// 优雅关闭：监听信号，收到后给 5 秒处理残余请求。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.InfoContext(startupCtx, "shutting down server...")

	ctx, cancel := context.WithTimeout(startupCtx, 5*time.Second)
	if pub != nil {
		if perr := pub.Shutdown(ctx); perr != nil {
			slog.ErrorContext(startupCtx, "public server shutdown", slog.String("error", perr.Error()))
		}
	}
	err = app.Shutdown(ctx)
	cancel() // 显式调用:下面的 os.Exit 不会执行 defer
	if err != nil {
		slog.ErrorContext(startupCtx, "server forced to shutdown", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.InfoContext(startupCtx, "server stopped")
}

// runPublicLogin 扫码登录服务账号(部署者自己的网易云账号),登录态写入文件,供公开模式取 VIP 播放链接。
// 容器里运行:docker exec -it kite server public-login --account primary。
func runPublicLogin(args []string) int {
	path, err := publicLoginTarget(args, config.Load().Public)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	eng := engine.New(engine.WithCache(cache.Noop{}))
	if err := public.Login(ctx, eng, path, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "登录失败:", err)
		return 1
	}
	return 0
}

// publicLoginTarget 在创建 engine 或请求上游前校验参数并确定唯一登录目标。
func publicLoginTarget(args []string, cfg config.PublicConfig) (string, error) {
	fs := flag.NewFlagSet("public-login", flag.ContinueOnError)
	file := fs.String("file", "", "显式登录态文件路径(不能与 --account 同用)")
	accountID := fs.String("account", "", "KITE_PUBLIC_ACCOUNTS 中配置的账号 ID")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() != 0 {
		return "", errors.New("public-login 不接受位置参数")
	}
	var fileSet, accountSet bool
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "file":
			fileSet = true
		case "account":
			accountSet = true
		}
	})
	if fileSet && accountSet {
		return "", errors.New("--account 与 --file 不能同时使用")
	}
	accounts, err := public.ParseServiceAccounts(cfg)
	if err != nil {
		return "", err
	}
	if fileSet {
		if strings.TrimSpace(*file) == "" {
			return "", errors.New("--file 不能为空")
		}
		return *file, nil
	}
	if accountSet {
		for _, account := range accounts {
			if account.ID == *accountID {
				return account.SessionFile, nil
			}
		}
		return "", fmt.Errorf("未配置服务账号 %q", *accountID)
	}
	if len(accounts) > 0 {
		return "", errors.New("配置 KITE_PUBLIC_ACCOUNTS 时必须使用 --account ID 或 --file 路径")
	}
	if path := strings.TrimSpace(cfg.SessionFile); path != "" {
		return path, nil
	}
	return "", errors.New("没有登录态文件路径:设置 KITE_PUBLIC_SESSION_FILE、使用 --account ID 或传 --file 路径")
}
