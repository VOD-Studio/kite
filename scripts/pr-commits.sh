#!/usr/bin/env bash
# 合并方式由维护者选择;所有非 merge 原子提交都应可供 release-please 正确推导版本。
# release PR 是人工发版闸门,只接受本仓库机器人创建的 main release 分支。
set -euo pipefail
GH=${GH:-gh}
: "${GH_REPO:?需要 GH_REPO}"
: "${PR_NUMBER:?需要 PR_NUMBER}"
: "${PR_BASE_SHA:?需要 CI plan 的 PR_BASE_SHA}"
: "${PR_HEAD_SHA:?需要 CI plan 的 PR_HEAD_SHA}"
[[ "$PR_NUMBER" =~ ^[0-9]+$ ]] || { echo 'PR_NUMBER 必须是数字' >&2; exit 1; }
pattern='^(feat|fix|perf|refactor|docs|style|test|chore|build|ci|revert|hotfix)(\([^()]+\))?(!)?: [^[:space:]].*'

pr=$("$GH" api "repos/$GH_REPO/pulls/$PR_NUMBER")
jq -e '.base.ref == "main" and (.commits | type == "number" and . > 0)' <<<"$pr" >/dev/null || {
  echo '::error::PR 必须指向 main,且提交数量有效'
  exit 1
}
title=$(jq -r '.title' <<<"$pr")
if ! [[ "$title" =~ $pattern ]] || [[ "$title" == *$'\n'* ]]; then
  echo '::error::PR 标题必须符合 Conventional Commits'
  exit 1
fi
if jq -e '.head.ref | startswith("release-please--")' <<<"$pr" >/dev/null; then
  PYTHONPATH="$(cd "$(dirname "$0")" && pwd)${PYTHONPATH:+:$PYTHONPATH}"
  export PYTHONPATH
  if ! python3 -c '
import json, os, sys
import release_common as common
pr = json.load(sys.stdin)
sys.exit(0 if os.environ["GH_REPO"] == common.REPO and any(common.trusted_release_pr(pr, component) for component in common.COMPONENTS) else 1)
' <<<"$pr"; then
    echo '::error::release PR 的仓库、机器人身份、组件分支或版本标题不匹配'
    exit 1
  fi
  echo 'release PR 应由维护者使用 Squash 合并;普通业务 PR 保留三种合并选项'
fi

python3 "$(dirname "$0")/pr-version-state.py" "$PR_BASE_SHA" "$PR_HEAD_SHA" <<<"$pr"

commits=$("$GH" api --paginate "repos/$GH_REPO/pulls/$PR_NUMBER/commits?per_page=100" | jq -s 'add')
expected=$(jq -r '.commits' <<<"$pr")
if [ "$(jq 'length' <<<"$commits")" != "$expected" ]; then
  echo '::error::GitHub 返回的提交列表不完整,不能确认全部提交的发布语义'
  exit 1
fi
failed=0
while IFS=$'\t' read -r sha headline; do
  if ! [[ "$headline" =~ $pattern ]]; then
    echo "::error::提交 $sha 不符合 Conventional Commits;请整理原子提交,无需强制 squash 整个 PR"
    failed=1
  fi
done < <(jq -r '.[] | select((.parents | length) < 2) | [.sha, (.commit.message | split("\n")[0])] | @tsv' <<<"$commits")
exit "$failed"
