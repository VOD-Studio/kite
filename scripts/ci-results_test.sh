#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
green='{"hygiene":{"result":"success"},"lint":{"result":"success"},"test":{"result":"success"},"cross-build":{"result":"success"},"proto":{"result":"success"},"vuln":{"result":"success"},"docker":{"result":"success"},"rust-interop":{"result":"success"},"go-interop":{"result":"success"},"release-check":{"result":"success"},"pr-title":{"result":"success"}}'
run_case() {
  local name=$1 event=$2 expected=$3 input=$4 code=0
  printf '%s\n' "$input" | bash "$here/ci-results.sh" "$event" >/dev/null 2>&1 || code=$?
  [ "$code" -eq "$expected" ] || { echo "FAIL $name: expected $expected got $code" >&2; exit 1; }
  echo "ok $name"
}
run_case 'complete PR passes' pull_request 0 "$green"
run_case 'push permits only PR job skip' push 0 "$(jq '."pr-title".result="skipped"' <<<"$green")"
run_case 'dispatch permits PR job skip' workflow_dispatch 0 "$(jq '."pr-title".result="skipped"' <<<"$green")"
run_case 'PR cannot skip title or commits' pull_request 1 "$(jq '."pr-title".result="skipped"' <<<"$green")"
for result in failure cancelled skipped; do
  run_case "required release check $result blocks" push 1 "$(jq --arg r "$result" '."release-check".result=$r' <<<"$green")"
done
run_case 'missing required job cannot greenlight release' pull_request 1 "$(jq 'del(.test)' <<<"$green")"
run_case 'new job must be classified before admission' pull_request 1 "$(jq '.unclassified={result:"success"}' <<<"$green")"
run_case 'malformed results fail closed' pull_request 1 '{}'
