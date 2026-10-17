#!/usr/bin/env bash
# scripts/publish-status.sh 的单元测试:用一个模拟的 violet MCP 服务端(python3 标准库)驱动脚本。
# 模拟服务端复刻了真实服务端的几条约束——这些约束正是脚本曾经踩过的坑:
#   - initialize 回 text/event-stream 并分配 Mcp-Session-Id;
#   - 之后每个请求必须带该会话 id,握手完成前的 tools/call 被拒;
#   - Accept 必须同时包含 application/json 与 text/event-stream;
#   - 必须有 Bearer 令牌。
# 用法:scripts/publish-status_test.sh
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
script="$root/scripts/publish-status.sh"
tmp=$(mktemp -d)
server_pid=""
stop_mock() {
  if [ -n "$server_pid" ]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
    server_pid=""
  fi
}
cleanup() {
  stop_mock
  rm -rf "$tmp"
}
trap cleanup EXIT

cat >"$tmp/mock.py" <<'PY'
import json, os, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

LOG = os.environ["MOCK_LOG"]
POST_ID = "post-1"
state = {
    "post": json.loads(os.environ["MOCK_POST"]),
    "session": "sess-abc",
    "initialized": False,
}
mode = os.environ.get("MOCK_MODE", "sse")
fail_update = os.environ.get("MOCK_FAIL_UPDATE") == "1"


def log(entry):
    with open(LOG, "a") as f:
        f.write(json.dumps(entry, ensure_ascii=False) + "\n")


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def reply(self, code, body=b"", ctype="application/json", extra=None):
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        for k, v in (extra or {}).items():
            self.send_header(k, v)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def rpc(self, msg_id, result=None, error=None, extra=None):
        msg = {"jsonrpc": "2.0", "id": msg_id}
        msg["error" if error else "result"] = error or result
        body = json.dumps(msg, ensure_ascii=False).encode()
        if mode == "sse":
            self.reply(200, b"event: message\ndata: " + body + b"\n\n", "text/event-stream", extra)
        else:
            self.reply(200, body, "application/json", extra)

    def do_POST(self):
        if not self.headers.get("Authorization", "").startswith("Bearer ") or len(self.headers["Authorization"]) < 10:
            return self.reply(401, b"no bearer token", "text/plain")
        accept = self.headers.get("Accept", "")
        if "application/json" not in accept or "text/event-stream" not in accept:
            return self.reply(400, b"Accept must contain both 'application/json' and 'text/event-stream'", "text/plain")
        msg = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        method = msg.get("method")

        if method == "initialize":
            state["initialized"] = False
            return self.rpc(msg["id"], {"protocolVersion": "2025-03-26", "capabilities": {}, "serverInfo": {"name": "mock"}},
                            extra={"Mcp-Session-Id": state["session"]})
        if self.headers.get("Mcp-Session-Id") != state["session"]:
            log({"rejected": method, "reason": "missing session"})
            return self.rpc(msg.get("id"), error={"code": 0, "message": 'method "%s" is invalid during session initialization' % method})
        if method == "notifications/initialized":
            state["initialized"] = True
            return self.reply(202)
        if method != "tools/call":
            return self.rpc(msg.get("id"), error={"code": -32601, "message": "unknown method"})

        name, args = msg["params"]["name"], msg["params"]["arguments"]
        text = lambda obj: {"content": [{"type": "text", "text": json.dumps(obj, ensure_ascii=False)}]}
        if name == "get_post":
            log({"call": "get_post"})
            return self.rpc(msg["id"], text(state["post"]))
        if name == "update_post":
            log({"call": "update_post", "args": args})
            if fail_update:
                return self.rpc(msg["id"], {"isError": True, "content": [{"type": "text", "text": "boom"}]})
            for k in ("title", "slug", "excerpt", "tags", "content_md", "cover_image", "show_signature"):
                if k in args:
                    state["post"][k] = args[k]
            return self.rpc(msg["id"], text(state["post"]))
        return self.rpc(msg["id"], {"isError": True, "content": [{"type": "text", "text": "unknown tool"}]})


srv = HTTPServer(("127.0.0.1", 0), H)
with open(os.environ["MOCK_PORT_FILE"], "w") as f:
    f.write(str(srv.server_address[1]))
srv.serve_forever()
PY

pass=0
fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "ok   $1"; pass=$((pass + 1)); }

printf '# 看板\n\n正文 | 含竖线\n' >"$tmp/progress.md"
cat >"$tmp/status.json" <<'JSON'
{"post":{"id":"post-1","slug":"kite-progress","title":"标题","excerpt":"摘要","tags":["developer-tools"]}}
JSON

# start_mock <线上文章 JSON> [环境变量...]
start_mock() {
  stop_mock
  rm -f "$tmp/port" "$tmp/log"
  : >"$tmp/log"
  local post=$1
  shift
  env MOCK_LOG="$tmp/log" MOCK_POST="$post" MOCK_PORT_FILE="$tmp/port" "$@" python3 "$tmp/mock.py" &
  server_pid=$!
  for _ in $(seq 1 50); do [ -s "$tmp/port" ] && break; sleep 0.1; done
  [ -s "$tmp/port" ] || fail "模拟服务没起来"
}

run() { # run [参数...]:返回脚本退出码,输出写到 $tmp/out
  set +e
  VIOLET_MCP_PAT="test-token-123456" VIOLET_MCP_URL="http://127.0.0.1:$(cat "$tmp/port")/mcp" \
    STATUS_JSON="$tmp/status.json" PROGRESS_MD="$tmp/progress.md" "$script" "$@" >"$tmp/out" 2>&1
  local rc=$?
  set -e
  return $rc
}

updates() { grep -c '"call": "update_post"' "$tmp/log" || true; }

# 线上与本地完全一致的文章
same_post() {
  jq -nc --rawfile md "$tmp/progress.md" \
    '{id:"post-1", slug:"kite-progress", title:"标题", excerpt:"摘要", tags:["developer-tools"], content_md:$md, cover_image:"https://x/c.png", show_signature:true, status:"published"}'
}

# 1. 内容一致 → 不更新(幂等)
start_mock "$(same_post)"
run || fail "一致时应成功: $(cat "$tmp/out")"
[ "$(updates)" = "0" ] || fail "内容一致却调用了 update_post"
grep -q "已是最新" "$tmp/out" || fail "应提示已是最新: $(cat "$tmp/out")"
ok "内容一致 → 不更新"

# 2. 正文不同 → 更新,且带回封面 / 署名开关,回读校验通过
start_mock "$(same_post | jq -c '.content_md = "旧内容\n"')"
run || fail "应更新成功: $(cat "$tmp/out")"
[ "$(updates)" = "1" ] || fail "应恰好更新一次"
upd=$(grep '"call": "update_post"' "$tmp/log" | jq -c .args)
jq -e --rawfile md "$tmp/progress.md" '.content_md == $md and .id == "post-1" and .title == "标题"' <<<"$upd" >/dev/null || fail "更新参数不对: $upd"
jq -e '.cover_image == "https://x/c.png" and .show_signature == true' <<<"$upd" >/dev/null || fail "整篇覆盖语义下封面 / 署名开关被清空: $upd"
ok "正文不同 → 更新一次,并保留封面与署名开关"

# 3. 标题 / 摘要 / 标签不同也要更新
start_mock "$(same_post | jq -c '.title = "旧标题"')"
run || fail "标题不同应更新: $(cat "$tmp/out")"
[ "$(updates)" = "1" ] || fail "标题不同却没更新"
ok "仅标题不同 → 更新"

# 3b. 仅 slug 不同也要更新(slug 决定文章 URL,幂等比较不能漏)
start_mock "$(same_post | jq -c '.slug = "old-slug"')"
run || fail "slug 不同应更新: $(cat "$tmp/out")"
[ "$(updates)" = "1" ] || fail "仅 slug 不同却没更新"
ok "仅 slug 不同 → 更新"

# 4. --dry-run:不写入
start_mock "$(same_post | jq -c '.content_md = "旧内容\n"')"
run --dry-run || fail "dry-run 应成功"
[ "$(updates)" = "0" ] || fail "dry-run 不应写入"
grep -q "dry-run" "$tmp/out" || fail "dry-run 应说明将更新"
ok "--dry-run 不写入"

# 5. --force:内容一致也更新
start_mock "$(same_post)"
run --force || fail "force 应成功: $(cat "$tmp/out")"
[ "$(updates)" = "1" ] || fail "--force 应更新"
ok "--force 内容一致也更新"

# 6. 服务端回 application/json(而非 SSE)也能工作
start_mock "$(same_post | jq -c '.content_md = "旧内容\n"')" MOCK_MODE=json
run || fail "json 响应应成功: $(cat "$tmp/out")"
[ "$(updates)" = "1" ] || fail "json 模式应更新"
ok "application/json 响应同样可用"

# 7. update_post 报错 → 脚本失败(不能吞掉错误)
start_mock "$(same_post | jq -c '.content_md = "旧内容\n"')" MOCK_FAIL_UPDATE=1
if run; then fail "update_post 返回错误时脚本应失败"; fi
grep -q "boom" "$tmp/out" || fail "应透出工具的错误信息: $(cat "$tmp/out")"
ok "update_post 出错 → 脚本失败并透出原因"

# 8. 文章 ID 对不上 → 失败(防止改错文章)
start_mock "$(same_post | jq -c '.id = "other"')"
if run; then fail "取回的文章 ID 不符时应失败"; fi
[ "$(updates)" = "0" ] || fail "ID 不符时不应更新"
ok "文章 ID 不符 → 失败且不写入"

# 9. 没有令牌 → 明确报错
start_mock "$(same_post)"
set +e
VIOLET_MCP_URL="http://127.0.0.1:$(cat "$tmp/port")/mcp" STATUS_JSON="$tmp/status.json" PROGRESS_MD="$tmp/progress.md" \
  env -u VIOLET_MCP_PAT "$script" >"$tmp/out" 2>&1
rc=$?
set -e
[ "$rc" != "0" ] || fail "缺少令牌应失败"
grep -q "VIOLET_MCP_PAT" "$tmp/out" || fail "应提示需要 VIOLET_MCP_PAT: $(cat "$tmp/out")"
ok "缺少令牌 → 明确报错"

# 10. 令牌不得出现在输出里
start_mock "$(same_post | jq -c '.content_md = "旧内容\n"')"
run || true
if grep -q "test-token-123456" "$tmp/out"; then fail "令牌泄露到了输出"; fi
ok "令牌不出现在输出里"

echo "全部通过($pass 项)"
