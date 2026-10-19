#!/usr/bin/env bash
# Go 互操作冒烟:证明「Go 项目能用 BSR 上生成的 SDK 接入这个服务」,并运行 docs/integration.md 里的示例。
#
#   1. interop/go 依赖 buf.build/gen/go 上由 kite proto 生成的 SDK(与接入方的真实路径一致);
#   2. 启动真实的 kite-server(开启访问令牌、回环、随机端口);
#   3. Go 测试经 gRPC 调用:错令牌得到 SERVER_AUTH_FAILED、没有网易云 cookie 得到 LOGIN_REQUIRED。
#
# 不访问上游网易云(缺 cookie 时服务端本地拒绝);需要访问 buf.build 下载 SDK。
# 依赖:go、python3(仅用于挑空闲端口)。
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

tmp=$(mktemp -d)
server_pid=""
cleanup() {
  [ -z "$server_pid" ] || kill "$server_pid" 2>/dev/null || true
  rm -rf "$tmp"
}
trap cleanup EXIT

go build -o "$tmp/server" ./cmd/server

port=$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
token=$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')

KITE_SERVER_GRPC_ADDR="127.0.0.1:${port}" \
KITE_SERVER_HTTP_ADDR="127.0.0.1:0" \
KITE_SERVER_AUTH_TOKEN="${token}" \
  "$tmp/server" >"$tmp/server.log" 2>&1 &
server_pid=$!

ready=""
for _ in $(seq 1 50); do
  if (exec 3<>"/dev/tcp/127.0.0.1/${port}") 2>/dev/null; then ready=1; break; fi
  kill -0 "$server_pid" 2>/dev/null || break
  sleep 0.2
done
if [ -z "$ready" ]; then
  echo "kite-server 没有就绪:" >&2
  cat "$tmp/server.log" >&2
  exit 1
fi

export GOPROXY=https://buf.build/gen/go,https://proxy.golang.org GONOSUMDB=buf.build/gen/go GOWORK=off
cd interop/go
go vet ./...
if ! KITE_GRPC_ADDR="127.0.0.1:${port}" KITE_TOKEN="${token}" go test -count=1 -v ./...; then
  echo "--- kite-server 日志 ---" >&2
  cat "$tmp/server.log" >&2
  exit 1
fi
