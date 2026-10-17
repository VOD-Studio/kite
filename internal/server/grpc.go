// Package server 装配 gRPC server 与 grpc-gateway runtime mux。
//
// gRPC server 监听 gRPC 端口对外提供强类型 RPC；gateway runtime mux 监听 HTTP 端口，
// 通过 dial 连本地 gRPC server，把 REST 请求转成 gRPC 调用。这是双 server 标准架构：
// 跨语言调用方走 gRPC，curl/Postman/前端走 gateway 暴露的 REST。
//
// 对外暴露面:bearer token 鉴权(auth.go)、标准 gRPC 健康检查与 REST /healthz、
// reflection。默认只监听回环地址,对外监听必须配置 token。
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/netease/engine"
	"github.com/VOD-Studio/kite/internal/netease/session"
	"github.com/VOD-Studio/kite/internal/service"

	runtime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
)

// gatewayHandlers 是全部领域 service 的 gateway REST handler 注册函数。
// 签名统一（func(ctx, mux, endpoint, dialOpts) error），可以表驱动。
// gRPC server 注册因各 Server 类型不同无法表驱动，用显式调用。
var gatewayHandlers = []func(context.Context, *runtime.ServeMux, string, []grpc.DialOption) error{
	mmpb.RegisterSongServiceHandlerFromEndpoint,
	mmpb.RegisterPlaylistServiceHandlerFromEndpoint,
	mmpb.RegisterAuthServiceHandlerFromEndpoint,
	mmpb.RegisterSearchServiceHandlerFromEndpoint,
	mmpb.RegisterAlbumServiceHandlerFromEndpoint,
	mmpb.RegisterArtistServiceHandlerFromEndpoint,
	mmpb.RegisterRecommendServiceHandlerFromEndpoint,
	mmpb.RegisterFMServiceHandlerFromEndpoint,
	mmpb.RegisterUserServiceHandlerFromEndpoint,
}

// Options 是服务的运行参数。
type Options struct {
	// GRPCAddr / HTTPAddr 是两个监听地址(host:port)。端口写 0 表示由系统分配(测试用),
	// 实际地址见 App.GRPCAddr / App.HTTPAddr。
	GRPCAddr string
	HTTPAddr string

	// AuthToken 非空时启用 bearer token 鉴权;为空时只允许监听回环地址。
	AuthToken string
}

// App 持有 gRPC server 与 gateway HTTP server，统一管理生命周期。
type App struct {
	grpcServer *grpc.Server
	httpServer *http.Server
	health     *health.Server
	grpcLis    net.Listener
	httpLis    net.Listener

	// stopGateway 结束 gateway 到 gRPC 的内部连接。
	stopGateway context.CancelFunc
}

// newGRPCServer 装配 gRPC server(拦截器链 + 全部领域 service + health + reflection),
// 不监听端口,便于测试。auth 为 nil 表示不鉴权。
//
// 拦截器顺序:ErrorInterceptor 在最外层,保证任何内层(含其它拦截器)返回的错误都被统一映射为
// gRPC status;其后 Auth 先于 Cookie(约定:未通过鉴权的请求不进入后续任何逻辑,
// 目前 Cookie 拦截器无副作用,这个顺序没有测试守护);CookieInterceptor 把 metadata 里的上游 cookie 注入 context。
// 链式为未来 recovery/trace/log/rate interceptor 留位。
func newGRPCServer(eng *engine.Engine, sessions session.SessionStore, auth *Auth) (*grpc.Server, *health.Server) {
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(ErrorInterceptor, auth.UnaryInterceptor, CookieInterceptor),
		grpc.ChainStreamInterceptor(auth.StreamInterceptor),
	)

	// 注册全部领域 service（真实 impl，持有 *engine.Engine）。
	mmpb.RegisterSongServiceServer(grpcServer, service.NewSongServer(eng))
	mmpb.RegisterPlaylistServiceServer(grpcServer, service.NewPlaylistServer(eng))
	mmpb.RegisterAuthServiceServer(grpcServer, service.NewAuthServer(eng, sessions))
	mmpb.RegisterSearchServiceServer(grpcServer, service.NewSearchServer(eng))
	mmpb.RegisterAlbumServiceServer(grpcServer, service.NewAlbumServer(eng))
	mmpb.RegisterArtistServiceServer(grpcServer, service.NewArtistServer(eng))
	mmpb.RegisterRecommendServiceServer(grpcServer, service.NewRecommendServer(eng))
	mmpb.RegisterFMServiceServer(grpcServer, service.NewFMServer(eng))
	mmpb.RegisterUserServiceServer(grpcServer, service.NewUserServer(eng))

	// 标准 gRPC 健康检查(grpc.health.v1):整体("")与每个已注册 service 各一项。
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	for name := range grpcServer.GetServiceInfo() {
		healthServer.SetServingStatus(name, healthpb.HealthCheckResponse_SERVING)
	}

	// reflection 让 grpcurl 可列出并调用全部 RPC;它是流式 RPC,同样受 Auth 保护。
	reflection.Register(grpcServer)
	return grpcServer, healthServer
}

// dialTarget 把监听地址转成本进程内可拨号的地址:监听全部网卡([::] / 0.0.0.0)时改拨回环。
func dialTarget(a net.Addr) string {
	host, port, err := net.SplitHostPort(a.String())
	if err != nil {
		return a.String()
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		if ip.To4() == nil {
			return net.JoinHostPort("::1", port)
		}
		return net.JoinHostPort("127.0.0.1", port)
	}
	return a.String()
}

// healthzHandler 是 REST 侧的存活探针:在进程内调用 gRPC 健康检查,SERVING 返回 200,否则 503。
// 不鉴权、不走 gRPC 网络栈,方便 LB / 容器探活。
func healthzHandler(h *health.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp, err := h.Check(r.Context(), &healthpb.HealthCheckRequest{})
		if err != nil || resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"NOT_SERVING"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"SERVING"}`))
	}
}

// NewApp 创建双 server 应用，注入 engine 和 session 池装配真实 service impl。
//
// 两个端口都在返回前完成监听:端口被占用等错误直接从这里返回,而不是启动后只写一行日志。
func NewApp(opts Options, eng *engine.Engine, sessions session.SessionStore) (*App, error) {
	auth := NewAuth(opts.AuthToken)
	if err := checkExposure(auth, opts.GRPCAddr, opts.HTTPAddr); err != nil {
		return nil, err
	}

	grpcLis, err := net.Listen("tcp", opts.GRPCAddr)
	if err != nil {
		return nil, fmt.Errorf("listen grpc %s: %w", opts.GRPCAddr, err)
	}
	httpLis, err := net.Listen("tcp", opts.HTTPAddr)
	if err != nil {
		_ = grpcLis.Close()
		return nil, fmt.Errorf("listen http %s: %w", opts.HTTPAddr, err)
	}

	grpcServer, healthServer := newGRPCServer(eng, sessions, auth)

	// gateway runtime mux：通过 dial 连本地 gRPC server。Authorization 头由 gateway 原样转成
	// gRPC metadata,鉴权统一在 gRPC 侧完成,REST 与 gRPC 不会出现两套规则。
	gwCtx, stopGateway := context.WithCancel(context.Background())
	gwMux := runtime.NewServeMux()
	muxOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	target := dialTarget(grpcLis.Addr())
	for _, h := range gatewayHandlers {
		if err := h(gwCtx, gwMux, target, muxOpts); err != nil {
			stopGateway()
			_ = grpcLis.Close()
			_ = httpLis.Close()
			return nil, fmt.Errorf("register gateway handler: %w", err)
		}
	}

	httpMux := http.NewServeMux()
	httpMux.Handle("GET /healthz", healthzHandler(healthServer))
	httpMux.Handle("/", gwMux)
	httpServer := &http.Server{
		Handler:      httpMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.InfoContext(context.Background(), "grpc server starting",
			slog.String("addr", grpcLis.Addr().String()), slog.Bool("auth", auth.Enabled()))
		if err := grpcServer.Serve(grpcLis); err != nil {
			slog.ErrorContext(context.Background(), "grpc server failed", slog.String("error", err.Error()))
		}
	}()
	go func() {
		slog.InfoContext(context.Background(), "gateway server starting",
			slog.String("addr", httpLis.Addr().String()), slog.Bool("auth", auth.Enabled()))
		if err := httpServer.Serve(httpLis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.ErrorContext(context.Background(), "gateway server failed", slog.String("error", err.Error()))
		}
	}()

	return &App{
		grpcServer:  grpcServer,
		httpServer:  httpServer,
		health:      healthServer,
		grpcLis:     grpcLis,
		httpLis:     httpLis,
		stopGateway: stopGateway,
	}, nil
}

// GRPCAddr 返回 gRPC 实际监听地址。
func (a *App) GRPCAddr() string { return a.grpcLis.Addr().String() }

// HTTPAddr 返回 REST 实际监听地址。
func (a *App) HTTPAddr() string { return a.httpLis.Addr().String() }

// Shutdown 优雅关闭:先把健康状态置为 NOT_SERVING(让 LB 摘流),再排空 REST,
// 最后排空 gRPC。ctx 到期时强制停止 gRPC,不会无限等待。
func (a *App) Shutdown(ctx context.Context) error {
	a.health.Shutdown()
	httpErr := a.httpServer.Shutdown(ctx)

	done := make(chan struct{})
	go func() {
		a.grpcServer.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		a.grpcServer.Stop()
		<-done
	}
	a.stopGateway()

	// 显式关监听器:Shutdown 若抢在 Serve 协程启动之前执行,Serve 感知不到停止,
	// 端口会一直开到协程被调度。重复关闭的错误无需处理。
	_ = a.grpcLis.Close()
	_ = a.httpLis.Close()
	return httpErr
}
