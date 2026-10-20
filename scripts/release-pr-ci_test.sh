#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/gh" <<'SHIM'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *'/pulls?state='*) printf '%s' "$PRS" ;;
  *'/actions/workflows/ci.yml'*) echo 5 ;;
  *'/actions/workflows/5/runs?'*) printf '%s' "$RUNS" ;;
  *'/pulls/7'*) printf '%s' "${CURRENT_HEAD:-abc}" ;;
  *'/approve'*) printf 'approve %s\n' "$*" >> "$CALLS" ;;
  'workflow run ci.yml --ref release-please--branches--main') echo dispatch >> "$CALLS" ;;
  *) echo "unexpected call: $*" >&2; exit 99 ;;
esac
SHIM
chmod +x "$tmp/gh"
export PATH="$tmp:$PATH" CALLS="$tmp/calls" GH_REPO=VOD-Studio/kite POLL_TRIES=1 POLL_SLEEP=0
trusted='[{"number":7,"user":{"login":"github-actions[bot]","type":"Bot"},"base":{"ref":"main","repo":{"full_name":"VOD-Studio/kite"}},"head":{"sha":"abc","ref":"release-please--branches--main","repo":{"full_name":"VOD-Studio/kite"}}}]'
run='{"id":9,"workflow_id":5,"head_sha":"abc","head_branch":"release-please--branches--main","head_repository":{"full_name":"VOD-Studio/kite"},"event":"pull_request","pull_requests":[{"number":7}],"conclusion":"action_required"}'
check() {
  local name=$1 expected=$2 expected_rc=${3:-0} rc=0 got
  : > "$CALLS"
  "$here/release-pr-ci.sh" > "$tmp/log" 2>&1 || rc=$?
  got=$(cat "$CALLS")
  [[ "$rc" == "$expected_rc" ]] || { cat "$tmp/log"; echo "FAIL $name exit=$rc"; exit 1; }
  case "$expected" in
    none) [[ -z "$got" ]] ;;
    approve) [[ "$got" == approve* && "$got" != *dispatch* ]] ;;
    dispatch) [[ "$got" == dispatch ]] ;;
  esac || { echo "FAIL $name calls=$got"; exit 1; }
  echo "ok $name"
}
export PRS="$trusted" RUNS="[{\"workflow_runs\":[$run]}]"
check 'trusted pending producer approved once' approve
RUNS="$(jq -c '.[0].workflow_runs[0].conclusion="success"' <<< "$RUNS")"
check 'successful exact producer is idempotent' none
RUNS="$(jq -c '.[0].workflow_runs[0].event="workflow_dispatch" | .[0].workflow_runs[0].pull_requests=[]' <<< "$RUNS")"
check 'dispatch recognized without redundant dispatch' none
export RUNS='[{"workflow_runs":[]}]'
check 'missing producer dispatches ci.yml' dispatch
export RUNS="[{\"workflow_runs\":[$run]}]"
RUNS="$(jq -c '.[0].workflow_runs[0].workflow_id=99' <<< "$RUNS")"
check 'AI review does not satisfy CI' dispatch
export RUNS="[{\"workflow_runs\":[$run]}]"
RUNS="$(jq -c '.[0].workflow_runs[0].head_sha="stale"' <<< "$RUNS")"
check 'old head does not satisfy CI' dispatch
export RUNS="[{\"workflow_runs\":[$run]}]"
PRS="$(jq -c '.[0].head.repo.full_name="attacker/kite"' <<< "$trusted")"
check 'same name fork cannot authorize' none
PRS="$(jq -c '.[0].base.ref="other"' <<< "$trusted")"
check 'wrong base cannot authorize' none
PRS="$(jq -c '.[0].user.login="human"' <<< "$trusted")"
check 'human branch cannot authorize' none
export PRS="$trusted" CURRENT_HEAD=changed
check 'head race cannot approve' none 1
export RUNS='[{"workflow_runs":[]}]'
check 'head race cannot dispatch' none 1
