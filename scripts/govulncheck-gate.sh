#!/usr/bin/env bash
# 可修复漏洞失败；仅明确 Fixed in: N/A 的完整报告允许 advisory。
# govulncheck 文本模式退出 3 表示发现漏洞，其余非零均为工具错误。
# 用法: scripts/govulncheck-gate.sh [包模式,默认 ./...]
# GITHUB_OUTPUT 的 advisory 供定时扫描通知；fixable 标明失败仅由可修复漏洞造成（非工具故障）。
# 本地也始终打印 warning。
set -euo pipefail

PKGS="${1:-./...}"
GOVULNCHECK="${GOVULNCHECK:-$(command -v govulncheck || echo "$(go env GOPATH)/bin/govulncheck")}"
if [ -n "${GITHUB_OUTPUT:-}" ]; then printf 'advisory=false\nfixable=false\n' >>"$GITHUB_OUTPUT"; fi
if [ ! -x "$GOVULNCHECK" ]; then
  echo "::error::未找到 govulncheck,跑 'make install-tools' 安装"
  exit 1
fi
code=0
output=$("$GOVULNCHECK" "$PKGS" 2>&1) || code=$?
printf '%s\n' "$output"
[ "$code" -eq 0 ] && exit 0
if [ "$code" -ne 3 ]; then
  echo "::error::govulncheck 执行失败(exit $code),扫描结果不可用"
  exit 1
fi

# 不根据本机 Go 版本猜测修复版是否可安装。报告缺失/格式变化也不得变绿。
counts=$(awk '
  /^[[:space:]]*Vulnerability #[0-9]+:/ { vulnerabilities++ }
  /^[[:space:]]*Fixed in:/ {
    fixes++
    if ($0 ~ /Fixed in:[[:space:]]*N\/A[[:space:]]*$/) advisory++
    else if ($0 ~ /Fixed in:[[:space:]]*[^[:space:]]+@[^[:space:]]+[[:space:]]*$/) fixable++
  }
  END { print vulnerabilities+0, fixes+0, advisory+0, fixable+0 }
' <<<"$output")
read -r vulnerabilities fixes advisory fixable <<<"$counts"
if [ "$vulnerabilities" -eq 0 ] || [ "$vulnerabilities" -ne "$fixes" ] || [ "$fixes" -ne "$((advisory + fixable))" ]; then
  echo '::error::govulncheck 漏洞报告不完整或无法识别,拒绝降级'
  exit 1
fi
if [ "$advisory" -gt 0 ]; then
  echo "::warning::$advisory 个漏洞尚无修复版本(Fixed in: N/A),需跟踪 advisory"
  if [ -n "${GITHUB_OUTPUT:-}" ]; then echo 'advisory=true' >>"$GITHUB_OUTPUT"; fi
fi
if [ "$fixable" -gt 0 ]; then
  level=error
  [ "${DEPENDENCY_SECURITY_MODE:-}" = report ] && level=warning
  echo "::$level::发现 $fixable 个可修复漏洞,请升级依赖或 Go toolchain"
  if [ -n "${GITHUB_OUTPUT:-}" ]; then echo 'fixable=true' >>"$GITHUB_OUTPUT"; fi
  exit 1
fi
