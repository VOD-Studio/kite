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
index=$(docker buildx imagetools inspect "$image:$TAG" --format '{{json .Manifest}}')
digest=$(jq -er '.digest | select(test("^sha256:[0-9a-f]{64}$"))' <<<"$index")
verified="$image@$digest"
cosign verify --certificate-identity-regexp "$identity" --certificate-oidc-issuer "$issuer" "$verified" > "$work/signature.json"
jq -e --arg digest "$digest" 'length > 0 and all(.[]; .critical.image["docker-manifest-digest"] == $digest)' "$work/signature.json" >/dev/null
# 逐架构校验必须按该架构自身的 manifest digest 拉取:用同一个 index digest 拉两次,
# docker 不允许把 repo@sha256:<index> 引用重新指向另一架构的镜像("cannot overwrite digest");
# 且 inspect index 引用永远只看到第一次拉到的架构,arm64 一轮会静默变成 amd64。
# 注意 buildx 模板输入没有 .Manifests 字段,子清单在 .Manifest(manifests 数组)里。
manifests=$(jq -er '.manifests' <<<"$index")
for arch in amd64 arm64; do
  arch_digest=$(jq -er --arg arch "$arch" 'map(select(.platform.os == "linux" and .platform.architecture == $arch)) | if length != 1 then error("linux/" + $arch + " manifest 数量: " + (length | tostring)) else .[0].digest end | select(test("^sha256:[0-9a-f]{64}$"))' <<<"$manifests")
  arch_ref="$image@$arch_digest"
  docker pull --platform "linux/$arch" "$arch_ref"
  labels=$(docker image inspect "$arch_ref" --format '{{json .Config.Labels}}')
  jq -e --arg sha "$SOURCE_SHA" --arg version "$version" '."org.opencontainers.image.revision" == $sha and ."org.opencontainers.image.version" == $version' <<< "$labels" >/dev/null
  "$root/docker-smoke.sh" "$arch_ref"
done
printf 'image=%s\n' "$verified" >> "$GITHUB_OUTPUT"
