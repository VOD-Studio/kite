// Package main 是 kite 服务的入口。
//
// 启动 gRPC server（对外强类型 RPC）+ grpc-gateway（REST 暴露）双 server，
// 收到 SIGINT/SIGTERM 时优雅关闭。
//
// 地基阶段：engine + session 池用 noop/cache 初始化（不接 Redis），
// 真实 Redis 接入在后续。
//
// 配置走环境变量(见 config.Load):默认只监听 127.0.0.1:3722(gRPC)与 127.0.0.1:3721(REST);
// 对外监听必须配置 KITE_SERVER_AUTH_TOKEN,否则拒绝启动。
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/VOD-Studio/kite/config"
	"github.com/VOD-Studio/kite/internal/cache"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
	"github.com/VOD-Studio/kite/internal/server"
)

// startupCtx 是启动阶段没有请求 ctx 时用的兜底 context。
var startupCtx = context.Background()

func main() {
	// 地基阶段：engine + session 池用 noop 初始化（不接 Redis）。
	// 真实接入在后续：WithCache(redis.New(rdb)) + WithSessions(store.NewSessionStore(rdb))。
	eng := engine.New(
		engine.WithCache(cache.Noop{}),
	)
	sessions := session.NoopStore{}

	cfg := config.Load()
	token, err := cfg.Server.ResolveAuthToken()
	if err != nil {
		slog.ErrorContext(startupCtx, "load auth token failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	app, err := server.NewApp(server.Options{
		GRPCAddr:  cfg.Server.GRPCAddr,
		HTTPAddr:  cfg.Server.HTTPAddr,
		AuthToken: token,
	}, eng, sessions)
	if err != nil {
		slog.ErrorContext(startupCtx, "init server failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.InfoContext(startupCtx, "server ready",
		slog.String("grpc", app.GRPCAddr()), slog.String("http", app.HTTPAddr()),
		slog.Bool("auth", token != ""))

	// 优雅关闭：监听信号，收到后给 5 秒处理残余请求。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.InfoContext(startupCtx, "shutting down server...")

	ctx, cancel := context.WithTimeout(startupCtx, 5*time.Second)
	err = app.Shutdown(ctx)
	cancel() // 显式调用:下面的 os.Exit 不会执行 defer
	if err != nil {
		slog.ErrorContext(startupCtx, "server forced to shutdown", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.InfoContext(startupCtx, "server stopped")
}
