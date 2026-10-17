# 本地开发 / 自部署用的多阶段构建(发布镜像见 Dockerfile.release,由 GoReleaser 构建)。
#
# GOPROXY 作为 build-arg:默认官方代理,CI 与海外环境直接可用;国内本地构建可覆盖:
#   docker build --build-arg GOPROXY=https://goproxy.cn,direct -t kite-server .

# 阶段1: 构建
FROM golang:1.26-alpine AS builder

ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

WORKDIR /app

# 先拷 go.mod / go.sum 单独下载依赖,利用层缓存(源码变化不重新下载)
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

# 阶段2: 运行时
# 与 Dockerfile.release 同一基础镜像:开发镜像与线上镜像一致,避免「本地能跑、发布不行」。
# static-debian12 自带 CA 证书,:nonroot 已是非 root 用户,无 shell,攻击面最小。
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /server /usr/local/bin/server

# 容器内必须监听全部网卡,端口映射才通。因此镜像不带令牌启动会被拒绝(这是有意的):
# 运行时必须设置 KITE_SERVER_AUTH_TOKEN 或 KITE_SERVER_AUTH_TOKEN_FILE。见 docs/server-security.md。
ENV KITE_SERVER_GRPC_ADDR=0.0.0.0:3722 \
    KITE_SERVER_HTTP_ADDR=0.0.0.0:3721

EXPOSE 3721 3722

ENTRYPOINT ["/usr/local/bin/server"]
