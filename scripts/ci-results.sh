#!/usr/bin/env bash
# 读取 GitHub needs JSON；只有非 PR 事件的 PR 专属 job 可以 skipped。
set -euo pipefail
event=${1:?用法: ci-results.sh <event-name> < needs.json}
results=$(cat)
jq -r 'to_entries[] | "\(.key): \(.value.result)"' <<<"$results"
if ! jq -e --arg event "$event" '
  ["hygiene", "lint", "test", "cross-build", "proto", "vuln", "docker", "rust-interop", "go-interop", "release-check", "pr-title"] as $required |
  (keys == ($required | sort)) and
  all(to_entries[];
    .value.result == "success" or
    (.key == "pr-title" and .value.result == "skipped" and
      ($event == "push" or $event == "workflow_dispatch")))
' <<<"$results" >/dev/null; then
  echo '::error::必要 CI job 缺失、未通过或出现未授权的 skip' >&2
  exit 1
fi
