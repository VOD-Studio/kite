#!/usr/bin/env bash
# 为部署 job 选 runner:组织级私有 runner(标签 omarchy)在线且空闲时用它,否则回退 GitHub 托管 runner。
# 结果写入 $GITHUB_OUTPUT 的 labels(runs-on 用的 JSON 数组),原因写入 $GITHUB_STEP_SUMMARY。
#
# 环境变量:
#   TOKEN      组织「Self-hosted runners」只读权限的令牌(仓库 secret RUNNER_STATUS_TOKEN);为空时直接回退。
#   ORG        组织名;RUNNER_ID 私有 runner 的 ID。
#   CURL       测试时覆盖。
set -euo pipefail

CURL="${CURL:-curl}"
fallback='["ubuntu-24.04"]'
preferred='["self-hosted","Linux","X64","omarchy"]'

pick() { # labels reason
  echo "labels=$1" >>"${GITHUB_OUTPUT}"
  echo "runner: $2" >>"${GITHUB_STEP_SUMMARY}"
  exit 0
}

[ -n "${TOKEN:-}" ] || pick "${fallback}" "未配置 RUNNER_STATUS_TOKEN,使用托管 runner"

info="$("${CURL}" -fsS --max-time 10 \
  -H "Authorization: Bearer ${TOKEN}" -H 'Accept: application/vnd.github+json' \
  "https://api.github.com/orgs/${ORG}/actions/runners/${RUNNER_ID}" 2>/dev/null)" || info=''

if jq -e '.status == "online" and .busy == false and ([.labels[].name] | index("omarchy") != null)' \
  <<<"${info}" >/dev/null 2>&1; then
  pick "${preferred}" "私有 runner ${RUNNER_ID} 在线且空闲"
fi
pick "${fallback}" "私有 runner ${RUNNER_ID} 不可用(离线 / 忙碌 / 查询失败),使用托管 runner"
