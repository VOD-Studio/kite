#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"
git init -q
git config user.name fixture
git config user.email fixture@example.invalid
mkdir gen
printf 'original\n' >gen/tracked.go
git add gen
git commit -qm fixture
assert_check() {
  local name=$1 want=$2 code=0
  bash "$here/check-generated.sh" gen >"$tmp/result" 2>&1 || code=$?
  [ "$code" -eq "$want" ] || { echo "FAIL $name: expected $want got $code" >&2; exit 1; }
  echo "ok $name"
}
assert_check 'clean generated tree passes' 0
printf 'new generated API\n' >gen/new.go
assert_check 'new untracked generated file blocks' 1
rm gen/new.go
printf 'modified\n' >gen/tracked.go
assert_check 'modified generated file blocks' 1
git add gen/tracked.go
assert_check 'staged generated file also blocks' 1
git commit -qm changed
rm gen/tracked.go
assert_check 'deleted generated file blocks' 1
