#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/source/proto"
cat > "$tmp/bin/gh" <<'SHIM'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *'/releases/tags/'*) printf '{"tag_name":"%s","draft":%s,"prerelease":false}' "$TAG" "${DRAFT:-false}" ;;
  *'/commits/'*) echo abc ;;
  *'/compare/'*) echo "${ANCESTRY:-ahead}" ;;
  *'/releases?per_page='*) printf '%s' "$RELEASES" ;;
  *) exit 99 ;;
esac
SHIM
cat > "$tmp/bin/buf" <<'SHIM'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$CALLS"
SHIM
chmod +x "$tmp/bin/"*
export PATH="$tmp/bin:$PATH" CALLS="$tmp/calls" GITHUB_OUTPUT="$tmp/out" GITHUB_REPOSITORY=VOD-Studio/kite GITHUB_REF=refs/heads/main TAG=v0.5.0 SOURCE_SHA=abc BUF_TOKEN=fake-test-token
export RELEASES='[[{"tag_name":"v0.5.0","draft":false,"prerelease":false},{"tag_name":"v0.10.0","draft":false,"prerelease":false},{"tag_name":"player-v99.0.0","draft":false,"prerelease":false}]]'
printf 'version: v2\nmodules:\n  - path: .\n' > "$tmp/source/proto/buf.yaml"
(cd "$tmp" && "$here/bsr-publish.sh" prepare)
grep -q '^sha=abc$' "$GITHUB_OUTPUT"
(cd "$tmp" && "$here/bsr-publish.sh" push)
grep -q -- '--label v0.5.0' "$CALLS"
if grep -q -- '--label main' "$CALLS"; then echo 'FAIL old version moved main'; exit 1; fi
grep -q 'name: buf.build/vod-studio/kite' "$tmp/source/proto/buf.yaml"
export TAG=v0.10.0
: > "$CALLS"
(cd "$tmp" && "$here/bsr-publish.sh" push)
grep -q -- '--label v0.10.0 --label main' "$CALLS"
for failure in draft fork source token tag player prerelease; do
  export DRAFT=false GITHUB_REPOSITORY=VOD-Studio/kite ANCESTRY=ahead BUF_TOKEN=fake-test-token TAG=v0.10.0
  case "$failure" in
    draft) export DRAFT=true ;;
    fork) export GITHUB_REPOSITORY=attacker/kite ;;
    source) export ANCESTRY=diverged ;;
    token) export BUF_TOKEN='' ;;
    tag) export TAG=v01.2.3 ;;
    player) export TAG=player-v99.0.0 ;;
    prerelease) export TAG=v1.0.0-rc.1 ;;
  esac
  : > "$CALLS"
  if (cd "$tmp" && "$here/bsr-publish.sh" push) > "$tmp/log" 2>&1; then echo "FAIL accepted $failure"; exit 1; fi
  [[ ! -s "$CALLS" ]] || { echo "FAIL pushed $failure"; exit 1; }
  echo "ok rejected $failure"
done
