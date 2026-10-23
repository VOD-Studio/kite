#!/usr/bin/env bash
# Reconstruct the plan from this event and complete Git diff before admitting skips.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
exec python3 "$here/ci-plan.py" check
