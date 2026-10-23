#!/usr/bin/env bash
# 有修复漏洞/工具故障失败；无修复版 advisory 聚合后供 CI 和定时通知消费。
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
exec python3 "$here/dependency-security.py" "$@"
