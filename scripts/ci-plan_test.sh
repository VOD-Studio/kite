#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
exec python3 "$here/ci-plan_test.py" PlanTests
