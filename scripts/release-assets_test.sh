#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/assets" "$tmp/tooling"
cp "$here/release-verify.sh" "$tmp/tooling/"
cat > "$tmp/tooling/docker-smoke.sh" <<'SHIM'
#!/usr/bin/env bash
[[ "${FAIL_SMOKE:-false}" != true ]]
SHIM
cat > "$tmp/bin/gh" <<'SHIM'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1 $2" == 'release download' ]] || exit 99
while (($#)); do
  if [[ "$1" == --dir ]]; then cp "$ASSETS/"* "$2/"; exit; fi
  shift
done
exit 99
SHIM
cat > "$tmp/bin/cosign" <<'SHIM'
#!/usr/bin/env bash
set -euo pipefail
[[ "${FAIL_SIGNATURE:-false}" != true ]] || exit 1
if [[ "$1" == verify ]]; then
  printf '[{"critical":{"image":{"docker-manifest-digest":"sha256:%s"}}}]' "$DIGEST"
fi
SHIM
cat > "$tmp/bin/docker" <<'SHIM'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  'buildx imagetools inspect '*)
    printf '{"digest":"sha256:%s","manifests":[{"platform":{"os":"linux","architecture":"amd64"},"digest":"sha256:%s"},{"platform":{"os":"linux","architecture":"arm64"},"digest":"sha256:%s"}]}' "$DIGEST" "${DIGEST//a/b}" "${DIGEST//a/c}" ;;
  'image inspect '*) printf '{"org.opencontainers.image.revision":"%s","org.opencontainers.image.version":"0.5.0"}' "${IMAGE_SHA:-abc}" ;;
  'pull '*) printf '%s\n' "$*" >> "$PULL_LOG"; : ;;
  *) exit 99 ;;
esac
SHIM
chmod +x "$tmp/bin/"* "$tmp/tooling/"*.sh
export PATH="$tmp/bin:$PATH" ASSETS="$tmp/assets" TAG=v0.5.0 SOURCE_SHA=abc GITHUB_OUTPUT="$tmp/output" PULL_LOG="$tmp/pulls.log"
export DIGEST=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
for prefix in kite kite-server; do
  for os in linux macOS windows; do
    for arch in amd64 arm64; do
      ext=tar.gz; [[ "$os" != windows ]] || ext=zip
      printf 'archive %s %s %s\n' "$prefix" "$os" "$arch" > "$ASSETS/${prefix}_0.5.0_${os}_${arch}.${ext}"
    done
  done
done
# Exercise the actual package builder with fake compiler output, not a second tar implementation.
mkdir -p "$tmp/source/scripts" "$tmp/source/dist/amd64" "$tmp/source/dist/arm64"
for file in deploy-server.sh deploy-ssh-gate.sh deploy-server-install.sh deploy-version; do echo content > "$tmp/source/scripts/$file"; done
for arch in amd64 arm64; do echo "$arch probe" > "$tmp/source/dist/$arch/kite-probe"; done
cat > "$tmp/source/dist/artifacts.json" <<'JSON'
[{"type":"Binary","goos":"linux","goarch":"amd64","extra":{"ID":"deploy-probe"},"path":"dist/amd64/kite-probe"},{"type":"Binary","goos":"linux","goarch":"arm64","extra":{"ID":"deploy-probe"},"path":"dist/arm64/kite-probe"}]
JSON
(cd "$tmp" && python3 "$here/release-package.py")
cp "$tmp/kite-deployer_v0.5.0.tar.gz" "$ASSETS/"
(cd "$ASSETS" && sha256sum kite_0.5.0_* kite-server_0.5.0_* > kite_0.5.0_checksums.txt)
echo bundle > "$ASSETS/kite_0.5.0_checksums.txt.sigstore.json"
echo bundle > "$ASSETS/kite-deployer_v0.5.0.tar.gz.sigstore.json"
"$tmp/tooling/release-verify.sh"
# Regression guard: each architecture must be pulled by its own manifest digest,
# never twice via the index digest (docker refuses to overwrite a digest ref).
[[ $(wc -l < "$PULL_LOG") -eq 2 ]] || { echo 'FAIL 应恰好按架构拉取两次'; exit 1; }
pulled=$(sed -E 's/.*@//' "$PULL_LOG" | LC_ALL=C sort -u)
[[ "$pulled" != *"sha256:$DIGEST"* && $(wc -l <<<"$pulled") -eq 2 ]] || { echo 'FAIL 未按各架构自身 digest 拉取'; exit 1; }
# An already-public application's original assets remain verifiable without a deployer.
rm "$ASSETS/kite-deployer_v0.5.0.tar.gz" "$ASSETS/kite-deployer_v0.5.0.tar.gz.sigstore.json"
"$tmp/tooling/release-verify.sh"
if REQUIRE_PACKAGE=true "$tmp/tooling/release-verify.sh" > "$tmp/log" 2>&1; then
  cat "$tmp/log"
  echo 'FAIL new draft accepted without deployer'; exit 1
fi
cp "$tmp/kite-deployer_v0.5.0.tar.gz" "$ASSETS/"
echo bundle > "$ASSETS/kite-deployer_v0.5.0.tar.gz.sigstore.json"
cp "$ASSETS/kite_0.5.0_checksums.txt" "$tmp/checksums"
for failure in unsigned signature revision smoke missing; do
  : > "$GITHUB_OUTPUT"
  cp "$tmp/checksums" "$ASSETS/kite_0.5.0_checksums.txt"
  export FAIL_SIGNATURE=false FAIL_SMOKE=false IMAGE_SHA=abc
  case "$failure" in
    unsigned) grep -v 'kite_0.5.0_windows_arm64.zip' "$tmp/checksums" > "$ASSETS/kite_0.5.0_checksums.txt" ;;
    signature) export FAIL_SIGNATURE=true ;;
    revision) export IMAGE_SHA=wrong ;;
    smoke) export FAIL_SMOKE=true ;;
    missing) rm "$ASSETS/kite_0.5.0_windows_arm64.zip" ;;
  esac
  if "$tmp/tooling/release-verify.sh" > "$tmp/log" 2>&1; then echo "FAIL accepted $failure"; exit 1; fi
  [[ ! -s "$GITHUB_OUTPUT" ]] || { echo "FAIL exported unverified image for $failure"; exit 1; }
  echo "ok rejected $failure"
done
