#!/usr/bin/env bash
set -euo pipefail
: "${TAG:?}" "${GITHUB_OUTPUT:?}"
[[ "${GITHUB_REPOSITORY:-}" == VOD-Studio/kite && "${GITHUB_REF:-}" == refs/heads/main ]] || exit 1
[[ "$TAG" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || exit 1
repo=VOD-Studio/kite
release=$(gh api "repos/$repo/releases/tags/$TAG")
jq -e --arg tag "$TAG" '.tag_name == $tag and .draft == false and .prerelease == false' <<< "$release" >/dev/null
sha=$(gh api "repos/$repo/commits/$TAG" --jq .sha)
comparison=$(gh api "repos/$repo/compare/$sha...main" --jq .status)
[[ "$comparison" == ahead || "$comparison" == identical ]] || exit 1
if [[ "$1" == prepare ]]; then
  printf 'sha=%s\n' "$sha" >> "$GITHUB_OUTPUT"
  exit 0
fi
[[ "$1" == push && "$sha" == "${SOURCE_SHA:?}" ]] || exit 1
[[ -n "${BUF_TOKEN:-}" ]] || { echo '::error::BUF_TOKEN missing; configure it and dispatch BSR on main to recover this version'; exit 1; }
# Older tags predate the BSR module name. Only metadata is added; proto is unchanged.
python3 - <<'PY'
from pathlib import Path
p = Path('source/proto/buf.yaml')
text = p.read_text()
if 'name: buf.build/' not in text:
    text = text.replace('  - path: .\n', '  - path: .\n    name: buf.build/vod-studio/kite\n')
    p.write_text(text)
if 'name: buf.build/vod-studio/kite' not in text:
    raise SystemExit('unexpected BSR module identity')
PY
latest=$(gh api --paginate --slurp "repos/$repo/releases?per_page=100" | jq -er '[.[][] | select(.draft == false and .prerelease == false) | .tag_name | select(test("^v(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)$"))] | max_by(ltrimstr("v") | split(".") | map(tonumber))')
labels=(--label "$TAG")
if [[ "$TAG" == "$latest" ]]; then labels+=(--label main); fi
buf push source/proto "${labels[@]}" --create --create-visibility public
