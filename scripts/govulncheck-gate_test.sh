#!/usr/bin/env bash
# 通过现有 GOVULNCHECK seam 验证退出状态与定时扫描可见的 advisory 信号。
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cat >"$tmp/govulncheck" <<'SHIM'
#!/usr/bin/env bash
printf '%s\n' "$REPORT"
exit "$SCAN_CODE"
SHIM
chmod +x "$tmp/govulncheck"
run_case() {
  local name=$1 expected=$2 scan_code=$3 report=$4 advisory=$5 code=0 actual
  : >"$tmp/outputs"
  GOVULNCHECK="$tmp/govulncheck" GITHUB_OUTPUT="$tmp/outputs" SCAN_CODE="$scan_code" REPORT="$report" \
    bash "$here/govulncheck-gate.sh" >"$tmp/log" 2>&1 || code=$?
  if [ "$code" -ne "$expected" ]; then
    echo "FAIL $name: expected $expected, got $code" >&2
    cat "$tmp/log" >&2
    exit 1
  fi
  actual=$(awk -F= '$1 == "advisory" { value=$2 } END { print value }' "$tmp/outputs")
  [ "$actual" = "$advisory" ] || { echo "FAIL $name: advisory expected $advisory, got $actual" >&2; exit 1; }
  if [ "$advisory" = true ]; then
    grep -q '::warning::' "$tmp/log" || { echo "FAIL $name: no visible warning" >&2; exit 1; }
  fi
  echo "ok $name"
}
run_case 'clean scan' 0 0 'No vulnerabilities found.' false
run_case 'newer Go fix must block rather than assume unavailable' 1 3 $'Vulnerability #1: GO-2099-0001\n    Fixed in: stdlib@go1.99.1' false
run_case 'dependency fix must block' 1 3 $'Vulnerability #1: GO-2099-0002\n    Fixed in: example.org/lib@v2.0.0' false
run_case 'N/A remains visible to scheduled scan' 0 3 $'Vulnerability #1: GO-2099-0003\n    Fixed in: N/A' true
run_case 'mixed N/A and fix must block and report advisory' 1 3 $'Vulnerability #1: GO-2099-0003\n    Fixed in: N/A\nVulnerability #2: GO-2099-0002\n    Fixed in: example.org/lib@v2.0.0' true
run_case 'compiler error fails closed' 1 1 'package cannot compile' false
run_case 'tool error containing partial vulnerability report fails closed' 1 1 $'Vulnerability #1: GO-2099-0003\n    Fixed in: N/A\nnetwork failure' false
run_case 'unrecognized finding fails closed' 1 3 'Vulnerability #1: GO-2099-0003' false
run_case 'truncated report fails closed' 1 3 $'Vulnerability #1: GO-2099-0003\n    Fixed in: N/A\nVulnerability #2: GO-2099-0004' false
