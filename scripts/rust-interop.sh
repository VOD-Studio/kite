#!/usr/bin/env bash
# Rust 互操作冒烟:证明「Rust 项目能用这个服务」。
#
#   1. buf 把 proto 编译成描述符(含 google.api 等依赖),Rust 的 build.rs 据此生成 tonic 客户端;
#   2. 启动真实的 kite-server(开启访问令牌、回环、随机 HTTP 端口);
#   3. Rust 测试经 gRPC 调用:健康检查免鉴权、无 / 错令牌得到 SERVER_AUTH_FAILED、
#      正确令牌到达业务层得到 LOGIN_REQUIRED(并校验 ErrorInfo 与 docs 里的 classify / 拦截器示例)。
#
# 不访问上游网易云(缺 cookie 时服务端本地拒绝),CI 无需外网访问网易云。
# 依赖:go、buf、cargo(工具链由 interop/rust/rust-toolchain.toml 固定)、python3(仅用于挑空闲端口)。
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

buf_bin=${BUF:-$(command -v buf || echo "$(go env GOPATH)/bin/buf")}
tmp=$(mktemp -d)
server_pid=""
cleanup() {
  [ -z "$server_pid" ] || kill "$server_pid" 2>/dev/null || true
  rm -rf "$tmp"
}
trap cleanup EXIT

"$buf_bin" build proto -o interop/rust/kite.binpb
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

if ! (cd interop/rust && KITE_GRPC_ADDR="127.0.0.1:${port}" KITE_TOKEN="${token}" \
  cargo test --locked -- --include-ignored); then
  echo "--- kite-server 日志 ---" >&2
  cat "$tmp/server.log" >&2
  exit 1
fi
