#!/usr/bin/env bash
# 镜像冒烟:验证镜像层面的安全默认值与探活,单元测试覆盖不到这一层(ENV、端口、入口)。
#
# 用法:scripts/docker-smoke.sh <image>
#   1. 不带令牌启动必须失败(镜像在容器内监听全部网卡,必须被令牌保护)。
#   2. 带令牌启动后:/healthz 不需要令牌即 200;业务 REST 无令牌 401 + SERVER_AUTH_FAILED。
# 不访问上游网易云,CI 无需外网。
set -euo pipefail

image="${1:?用法: $0 <image>}"
token="smoke-$$"
cid=""
cleanup() { [ -z "$cid" ] || docker rm -f "$cid" >/dev/null 2>&1 || true; }
trap cleanup EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }

# 1. 无令牌必须拒绝启动。后台启动并轮询状态:若它(错误地)启动成功,不会让脚本一直挂住。
cid=$(docker run -d "$image")
state=""
for _ in $(seq 1 20); do
  state=$(docker inspect -f '{{.State.Status}}' "$cid")
  [ "$state" = "exited" ] && break
  sleep 0.5
done
[ "$state" = "exited" ] || fail "不带令牌竟然启动成功了(状态 $state)"
[ "$(docker inspect -f '{{.State.ExitCode}}' "$cid")" != "0" ] || fail "拒绝启动应以非 0 退出"
logs=$(docker logs "$cid" 2>&1)
grep -q "KITE_SERVER_AUTH_TOKEN" <<<"$logs" || fail "拒绝启动但错误信息没有指出如何配置令牌: $logs"
docker rm -f "$cid" >/dev/null
cid=""
echo "ok   无令牌 → 拒绝启动"

# 2. 带令牌启动(端口只发布到宿主机回环,随机端口避免冲突)
cid=$(docker run -d -e KITE_SERVER_AUTH_TOKEN="$token" -p 127.0.0.1::3721 "$image")
port=$(docker port "$cid" 3721/tcp | head -1 | awk -F: '{print $NF}')
[ -n "$port" ] || fail "取不到映射端口"

up=""
for _ in $(seq 1 30); do
  if body=$(curl -fsS -m 2 "http://127.0.0.1:${port}/healthz" 2>/dev/null); then up=1; break; fi
  sleep 0.5
done
[ -n "$up" ] || { docker logs "$cid" >&2; fail "/healthz 15 秒内没有就绪"; }
grep -q '"SERVING"' <<<"$body" || fail "/healthz 响应异常: $body"
echo "ok   带令牌 → /healthz 200 SERVING(无需令牌)"

raw=$(curl -s -m 5 -w '\n%{http_code}' "http://127.0.0.1:${port}/api/v1/songs/1")
code=${raw##*$'\n'}
resp=${raw%$'\n'*}
[ "$code" = "401" ] || fail "无令牌访问业务 REST 应为 401,实际 $code: $resp"
grep -q "SERVER_AUTH_FAILED" <<<"$resp" || fail "401 响应缺少 SERVER_AUTH_FAILED: $resp"
echo "ok   业务 REST 无令牌 → 401 SERVER_AUTH_FAILED"
