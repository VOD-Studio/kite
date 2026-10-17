#!/usr/bin/env bash
# 把 docs/status/progress.md 同步到 violet 博客里的「kite 开发进度看板」文章(经 violet 的 MCP 接口)。
#
# **手动工具,不接 CI**:只在维护者明确要求同步时才运行(博客是对外内容,更新即改线上)。
# 令牌不存进仓库 secret,由运行者在本地提供。
#
# 用法:VIOLET_MCP_PAT=<令牌> scripts/publish-status.sh [--dry-run] [--force]
#   --dry-run  只读:取文章、比较、报告会不会更新,不写入
#   --force    内容没变化也执行更新(用来验证写入链路)
#
# 行为:
#   - 文章 ID 取自 docs/status/status.json 的 post.id。首次创建 / 发布由人工完成,脚本只更新已存在的文章;
#   - 正文、标题、链接 slug、摘要、标签与线上完全一致则什么也不做(幂等,重复运行不产生新版本);
#   - 更新是整篇覆盖语义:未改动的字段(封面、署名开关)从线上原值带回,不会被清空。
#
# 依赖:curl、jq。令牌只经 0600 临时文件交给 curl,不出现在进程参数里。
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
status_json="${STATUS_JSON:-$root/docs/status/status.json}"
body_md="${PROGRESS_MD:-$root/docs/status/progress.md}"
url="${VIOLET_MCP_URL:-https://xunrua.top/api/v1/mcp}"

dry_run=""
force=""
for arg in "$@"; do
  case "$arg" in
    --dry-run) dry_run=1 ;;
    --force) force=1 ;;
    *) echo "未知参数: $arg" >&2; exit 2 ;;
  esac
done

: "${VIOLET_MCP_PAT:?需要环境变量 VIOLET_MCP_PAT(violet 的 MCP 访问令牌,至少 posts:read 与 posts:write)}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
chmod 700 "$tmp"
umask 077
printf 'Authorization: Bearer %s\n' "$VIOLET_MCP_PAT" >"$tmp/auth"

post_id=$(jq -er '.post.id' "$status_json")
title=$(jq -er '.post.title' "$status_json")
excerpt=$(jq -er '.post.excerpt' "$status_json")
tags=$(jq -ec '.post.tags' "$status_json")
slug=$(jq -er '.post.slug' "$status_json")

session=""
req_id=0

# mcp <json-rpc 请求体>:发送请求,把响应里的 JSON-RPC 消息写入 $tmp/msg。
# 必须在父 shell 里直接调用(不能放进 $(...)):会话 id 要写回全局变量 session,子 shell 里的赋值会丢失,
# 服务端就会把后续请求当成「尚未完成初始化的新会话」而拒绝。
# 服务端可能回 application/json,也可能回 text/event-stream(取最后一条 data:)。
mcp() {
  local payload=$1 code sid
  local headers=(-H "@$tmp/auth" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream")
  [ -z "$session" ] || headers+=(-H "Mcp-Session-Id: $session")
  code=$(curl -sS --max-time 60 -o "$tmp/resp" -D "$tmp/resp.h" -w '%{http_code}' "${headers[@]}" -d "$payload" "$url")
  if [ "${code:0:1}" != "2" ]; then
    echo "MCP 请求失败: HTTP $code" >&2
    head -c 400 "$tmp/resp" >&2 || true
    echo >&2
    return 1
  fi
  sid=$(awk 'tolower($1)=="mcp-session-id:"{gsub("\r","",$2); print $2}' "$tmp/resp.h" | tail -1)
  [ -z "$sid" ] || session=$sid
  if grep -q '^data:' "$tmp/resp"; then
    grep '^data:' "$tmp/resp" | tail -1 | sed 's/^data: *//' >"$tmp/msg"
  else
    cp "$tmp/resp" "$tmp/msg"
  fi
}

# tool <名字> <arguments JSON>:调用工具,成功时把第一段文本内容写到 stdout(可放进 $(...))。
tool() {
  req_id=$((req_id + 1))
  local msg
  msg=$(jq -nc --argjson id "$req_id" --arg name "$1" --argjson args "$2" \
    '{jsonrpc:"2.0", id:$id, method:"tools/call", params:{name:$name, arguments:$args}}')
  mcp "$msg" || return 1
  if jq -e '.error' "$tmp/msg" >/dev/null 2>&1; then
    echo "MCP 错误($1): $(jq -c '.error' "$tmp/msg")" >&2
    return 1
  fi
  if jq -e '.result.isError == true' "$tmp/msg" >/dev/null 2>&1; then
    echo "工具 $1 返回错误: $(jq -r '.result.content[0].text // "未知"' "$tmp/msg" | head -c 400)" >&2
    return 1
  fi
  jq -er '.result.content[0].text' "$tmp/msg"
}

mcp '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"kite-publish-status","version":"1"}}}'
jq -e '.result' "$tmp/msg" >/dev/null || { echo "initialize 失败: $(head -c 300 "$tmp/msg")" >&2; exit 1; }
mcp '{"jsonrpc":"2.0","method":"notifications/initialized"}' || true

current=$(tool get_post "$(jq -nc --arg id "$post_id" '{id:$id}')")
jq -e --arg id "$post_id" '.id == $id' >/dev/null <<<"$current" || { echo "取回的文章与 post.id 不符" >&2; exit 1; }

same=$(jq --rawfile md "$body_md" --arg title "$title" --arg slug "$slug" --arg excerpt "$excerpt" --argjson tags "$tags" \
  '(.content_md == $md) and (.title == $title) and (.slug == $slug) and (.excerpt == $excerpt) and ((.tags // []) == $tags)' <<<"$current")

if [ "$same" = "true" ] && [ -z "$force" ]; then
  echo "进度看板已是最新,无需更新(文章 $post_id)"
  exit 0
fi

if [ -n "$dry_run" ]; then
  echo "[dry-run] 线上内容与 docs/status 不一致,将更新文章 $post_id"
  exit 0
fi

args=$(jq -nc --arg id "$post_id" --arg title "$title" --arg slug "$slug" --arg excerpt "$excerpt" \
  --argjson tags "$tags" --rawfile md "$body_md" --argjson cur "$current" \
  '{id:$id, title:$title, slug:$slug, excerpt:$excerpt, tags:$tags, content_md:$md,
    cover_image:($cur.cover_image // ""), show_signature:($cur.show_signature // false)}')
updated=$(tool update_post "$args")
jq -e --arg id "$post_id" '.id == $id' >/dev/null <<<"$updated" || { echo "update_post 返回异常" >&2; exit 1; }

# 回读校验:写入后线上内容必须等于本地生成物,而不是只相信「调用成功」。
verify=$(tool get_post "$(jq -nc --arg id "$post_id" '{id:$id}')")
jq -e --rawfile md "$body_md" '.content_md == $md' >/dev/null <<<"$verify" || {
  echo "更新后回读的正文与本地不一致" >&2
  exit 1
}
echo "进度看板已更新(文章 $post_id)"
