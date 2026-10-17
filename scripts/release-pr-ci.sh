#!/usr/bin/env bash
# 保证 release-please 的 release PR 头部提交上有一个会完成的 CI 运行。
# 背景与取舍见 .github/workflows/release-please.yml 中调用处的注释。
#
# 环境变量:
#   BRANCH       release PR 的分支名(必填)
#   GH_REPO      owner/repo(gh 用)
#   GH_TOKEN     令牌(需 actions: write)
#   POLL_TRIES   轮询次数,默认 6
#   POLL_SLEEP   轮询间隔秒数,默认 10
set -euo pipefail

: "${BRANCH:?需要 BRANCH}"
tries="${POLL_TRIES:-6}"
nap="${POLL_SLEEP:-10}"

head=$(gh pr list --head "${BRANCH}" --state open --json headRefOid --jq '.[0].headRefOid // empty')
if [ -z "${head}" ]; then
  echo "没有打开的 release PR,跳过"
  exit 0
fi

for i in $(seq 1 "${tries}"); do
  # 每行:<run id> <conclusion>(进行中的运行 conclusion 为空)
  runs=$(gh run list --branch "${BRANCH}" --event pull_request --limit 20 \
    --json databaseId,headSha,conclusion \
    --jq ".[] | select(.headSha == \"${head}\") | \"\(.databaseId) \(.conclusion)\"")
  if [ -n "${runs}" ]; then
    while read -r id conclusion; do
      if [ "${conclusion}" = "action_required" ]; then
        echo "批准等待批准的运行 ${id}"
        gh api -X POST "repos/${GH_REPO}/actions/runs/${id}/approve" >/dev/null
      fi
    done <<<"${runs}"
    echo "head ${head:0:7} 上已有 pull_request 运行,无需 dispatch"
    exit 0
  fi
  echo "[${i}/${tries}] head ${head:0:7} 上暂无 pull_request 运行,${nap} 秒后重试"
  sleep "${nap}"
done

echo "::warning::head ${head:0:7} 上始终没有 pull_request 运行(GITHUB_TOKEN 触发的事件不会触发其它 workflow),回退到 workflow_dispatch"
gh workflow run ci.yml --ref "${BRANCH}"
