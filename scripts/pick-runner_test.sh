#!/usr/bin/env bash
# scripts/pick-runner.sh 的单元测试:用假的 curl 返回 runner 状态,断言选出的 labels。
# 运行: scripts/pick-runner_test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

cat >"${tmp}/curl" <<'SHIM'
#!/usr/bin/env bash
[ -n "${FAKE_INFO}" ] || exit 22
printf '%s' "${FAKE_INFO}"
SHIM
chmod +x "${tmp}/curl"

fail=0
run_case() { # name token info want_labels
  : >"${tmp}/out"
  TOKEN="$2" FAKE_INFO="$3" CURL="${tmp}/curl" ORG=o RUNNER_ID=1 \
    GITHUB_OUTPUT="${tmp}/out" GITHUB_STEP_SUMMARY=/dev/null "${here}/pick-runner.sh"
  local got
  got="$(sed -n 's/^labels=//p' "${tmp}/out")"
  if [ "${got}" = "$4" ]; then echo "ok   $1"; else echo "FAIL $1: 期望 $4 实际 ${got}"; fail=1; fi
}

self='["self-hosted","Linux","X64","omarchy"]'
hosted='["ubuntu-24.04"]'
labels='"labels":[{"name":"self-hosted"},{"name":"omarchy"}]'

run_case "在线且空闲 → 私有 runner" t "{\"status\":\"online\",\"busy\":false,${labels}}" "${self}"
run_case "忙碌 → 托管 runner"       t "{\"status\":\"online\",\"busy\":true,${labels}}" "${hosted}"
run_case "离线 → 托管 runner"       t "{\"status\":\"offline\",\"busy\":false,${labels}}" "${hosted}"
run_case "标签不符 → 托管 runner"   t '{"status":"online","busy":false,"labels":[{"name":"self-hosted"}]}' "${hosted}"
run_case "查询失败 → 托管 runner"   t "" "${hosted}"
run_case "未配置令牌 → 托管 runner" "" "{\"status\":\"online\",\"busy\":false,${labels}}" "${hosted}"

exit "${fail}"
