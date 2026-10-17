#!/usr/bin/env bash
# scripts/release-pr-ci.sh 的单元测试:用假的 gh 覆盖四种场景,断言它调用了哪些 gh 子命令。
# 运行: scripts/release-pr-ci_test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

# 假 gh:按子命令返回预置输出,并把「有副作用的调用」记到 calls.log。
cat >"${tmp}/gh" <<'SHIM'
#!/usr/bin/env bash
case "$1 $2" in
  "pr list")  printf '%s' "${FAKE_HEAD-}" ;;
  "run list") printf '%s' "${FAKE_RUNS-}" ;;
  "api -X")   echo "approve $*" >>"${CALLS}" ;;
  "workflow run") echo "dispatch $*" >>"${CALLS}" ;;
  *) echo "unexpected gh call: $*" >&2; exit 99 ;;
esac
SHIM
chmod +x "${tmp}/gh"

fail=0
run_case() { # name expected_calls_regex(空=无调用) HEAD RUNS
  local name="$1" want="$2"
  : >"${tmp}/calls.log"
  PATH="${tmp}:${PATH}" CALLS="${tmp}/calls.log" FAKE_HEAD="$3" FAKE_RUNS="$4" \
    BRANCH=release-please--branches--main GH_REPO=o/r POLL_TRIES=2 POLL_SLEEP=0 \
    "${here}/release-pr-ci.sh" >"${tmp}/out.log" 2>&1
  local rc=$? got
  got="$(cat "${tmp}/calls.log")"
  if [ "${rc}" -ne 0 ]; then echo "FAIL ${name}: 退出码 ${rc}"; cat "${tmp}/out.log"; fail=1; return; fi
  if [ -z "${want}" ]; then
    [ -z "${got}" ] || { echo "FAIL ${name}: 期望无调用,实际: ${got}"; fail=1; return; }
  else
    grep -Eq "${want}" <<<"${got}" || { echo "FAIL ${name}: 期望匹配 ${want},实际: ${got}"; fail=1; return; }
  fi
  echo "ok   ${name}"
}

run_case "没有打开的 release PR → 不做任何事"      ""                      ""      ""
run_case "有等待批准的运行 → 批准,不 dispatch"     "^approve .*runs/123/"  "abc123" $'123 action_required'
run_case "已有成功运行 → 无需任何操作"             ""                      "abc123" $'124 success'
run_case "运行进行中(conclusion 为空) → 无需操作" ""                      "abc123" $'125 '
run_case "始终没有运行 → 回退 dispatch"            "^dispatch .*ci.yml"    "abc123" ""

# 额外断言:有待批准运行时绝不能同时 dispatch(会产生重复运行)
: >"${tmp}/calls.log"
PATH="${tmp}:${PATH}" CALLS="${tmp}/calls.log" FAKE_HEAD=abc123 FAKE_RUNS=$'1 action_required' \
  BRANCH=b GH_REPO=o/r POLL_TRIES=1 POLL_SLEEP=0 "${here}/release-pr-ci.sh" >/dev/null 2>&1
if grep -q '^dispatch' "${tmp}/calls.log"; then echo "FAIL 批准时不应 dispatch"; fail=1; else echo "ok   批准时不 dispatch"; fi

exit "${fail}"
