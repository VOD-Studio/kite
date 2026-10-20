#!/usr/bin/env bash
# Verify downloaded bytes and smoke the exact pushed multi-architecture image.
set -euo pipefail
: "${TAG:?}" "${SOURCE_SHA:?}" "${GITHUB_OUTPUT:?}"
repo=VOD-Studio/kite
image=ghcr.io/vod-studio/kite-server
identity='^https://github\.com/VOD-Studio/kite/\.github/workflows/(release|release-please)\.yml@refs/heads/main$'
issuer=https://token.actions.githubusercontent.com
root=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
gh release download "$TAG" --repo "$repo" --dir "$work"
version=${TAG#v}
for prefix in kite kite-server; do
  for os in linux macOS windows; do
    for arch in amd64 arm64; do
      ext=tar.gz
      [[ "$os" != windows ]] || ext=zip
      test -s "$work/${prefix}_${version}_${os}_${arch}.${ext}"
    done
  done
done
checksums="kite_${version}_checksums.txt"
cosign verify-blob --bundle "$work/${checksums}.sigstore.json" \
  --certificate-identity-regexp "$identity" --certificate-oidc-issuer "$issuer" "$work/$checksums"
# Every required archive must be covered by the signed checksum list, not merely present.
python3 - "$work/$checksums" "$version" <<'PY'
import re
import sys
from pathlib import Path
entries = {}
for line in Path(sys.argv[1]).read_text().splitlines():
    match = re.fullmatch(r"([0-9a-f]{64}) [ *]([^/\\]+)", line)
    if not match or match[2] in entries:
        raise SystemExit("invalid or duplicate checksum entry")
    entries[match[2]] = match[1]
for prefix in ("kite", "kite-server"):
    for platform in ("linux", "macOS", "windows"):
        for arch in ("amd64", "arm64"):
            ext = "zip" if platform == "windows" else "tar.gz"
            name = f"{prefix}_{sys.argv[2]}_{platform}_{arch}.{ext}"
            if name not in entries:
                raise SystemExit(f"unsigned archive: {name}")
PY
(cd "$work" && sha256sum --check "$checksums")
package="kite-deployer_${TAG}.tar.gz"
if [[ "${REQUIRE_PACKAGE:-false}" == true || -e "$work/$package" || -e "$work/${package}.sigstore.json" ]]; then
  [[ -s "$work/$package" && -s "$work/${package}.sigstore.json" ]] || {
    echo '发布安装包或签名 bundle 缺失' >&2
    exit 1
  }
  cosign verify-blob --bundle "$work/${package}.sigstore.json" \
    --certificate-identity-regexp "$identity" --certificate-oidc-issuer "$issuer" "$work/$package"
  expected=$'deploy-server-install.sh\ndeploy-server.sh\ndeploy-ssh-gate.sh\ndeploy-version\nlinux/amd64/kite-probe\nlinux/arm64/kite-probe'
  members=$(tar -tzf "$work/$package" | LC_ALL=C sort)
  [[ "$members" == "$expected" ]] || {
    echo '发布安装包成员不完整或包含额外文件' >&2
    exit 1
  }
fi
digest=$(docker buildx imagetools inspect "$image:$TAG" --format '{{json .Manifest}}' | jq -er '.digest | select(test("^sha256:[0-9a-f]{64}$"))')
verified="$image@$digest"
cosign verify --certificate-identity-regexp "$identity" --certificate-oidc-issuer "$issuer" "$verified" > "$work/signature.json"
jq -e --arg digest "$digest" 'length > 0 and all(.[]; .critical.image["docker-manifest-digest"] == $digest)' "$work/signature.json" >/dev/null
for arch in amd64 arm64; do
  export DOCKER_DEFAULT_PLATFORM="linux/$arch"
  docker pull --platform "$DOCKER_DEFAULT_PLATFORM" "$verified"
  labels=$(docker image inspect "$verified" --format '{{json .Config.Labels}}')
  jq -e --arg sha "$SOURCE_SHA" --arg version "$version" '."org.opencontainers.image.revision" == $sha and ."org.opencontainers.image.version" == $version' <<< "$labels" >/dev/null
  "$root/docker-smoke.sh" "$verified"
done
printf 'image=%s\n' "$verified" >> "$GITHUB_OUTPUT"
