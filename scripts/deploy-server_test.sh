#!/usr/bin/env bash
# 真实事务的行为测试:替换外部系统,保留真实 jq/flock/状态文件。
# 依赖 Linux GNU 工具;CI 必跑,不使用容器运行时或网络。
set -uo pipefail
if [[ "$(uname -s)" != Linux ]]; then
  echo 'skip: 部署事务测试需要 Linux;CI 的 Hygiene job 执行完整测试'
  exit 0
fi
here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT
mkdir -p "${tmp}/bin" "${tmp}/installed"
cp "${here}/deploy-server.sh" "${here}/deploy-version" "${tmp}/installed/"
printf '#!/bin/sh\nexit 0\n' >"${tmp}/installed/kite-probe"
chmod 0755 "${tmp}/installed/kite-probe"
printf '%064d\n' 0 >"${tmp}/installed/deploy-build"
chmod 0644 "${tmp}/installed/deploy-build"
REAL_STAT="$(command -v stat)"
REAL_UNAME="$(command -v uname)"
FAKE_PLATFORM="sha256:$(printf 'a%.0s' {1..64})"
FAKE_ARM_PLATFORM="sha256:$(printf 'd%.0s' {1..64})"
FAKE_OLD="sha256:$(printf 'b%.0s' {1..64})"
FAKE_COMMIT="$(printf 'c%.0s' {1..40})"
export REAL_STAT REAL_UNAME FAKE_PLATFORM FAKE_ARM_PLATFORM FAKE_OLD FAKE_COMMIT
printf '{"schemaVersion":2,"manifests":[{"digest":"%s","platform":{"os":"linux","architecture":"amd64"}},{"digest":"%s","platform":{"os":"linux","architecture":"arm64"}}]}\n' "${FAKE_PLATFORM}" "${FAKE_ARM_PLATFORM}" >"${tmp}/manifest.json"
read -r hash _ < <(sha256sum "${tmp}/manifest.json")
export FAKE_INDEX="sha256:${hash}" FAKE_MANIFEST="${tmp}/manifest.json"
export STATE_DIR="${tmp}/state" CALLS="${tmp}/calls" PATH="${tmp}/bin:${PATH}"
export HEALTH_TRIES=1 HEALTH_SLEEP=0
cat >"${tmp}/bin/uname" <<'SHIM'
#!/usr/bin/env bash
if [[ "${1:-}" == -m ]]; then printf '%s\n' "${FAKE_ARCH:-x86_64}"; else exec "${REAL_UNAME}" "$@"; fi
SHIM
cat >"${tmp}/bin/stat" <<'SHIM'
#!/usr/bin/env bash
# Model root ownership for CI's non-root test account; preserve real permissions.
case "$2" in
  %u:%a) printf '0:%s\n' "$("${REAL_STAT}" -c %a "$3")" ;;
  *) exec "${REAL_STAT}" "$@" ;;
esac
SHIM
cat >"${tmp}/bin/podman" <<'SHIM'
#!/usr/bin/env bash
printf 'podman %s\n' "$*" >>"${CALLS}"
case "$1" in
  info) exit 0 ;;
  secret) [[ "${FAKE_SECRET_FAIL:-}" != 1 ]] ;;
  network)
    [[ "$2" != inspect ]] || printf '[{"dns_enabled":%s}]\n' "${FAKE_DNS:-true}"
    ;;
  container) [[ -e "${STATE_DIR}/current" ]] ;;
  inspect)
    case "$3" in
      '{{.Image}}') printf 'old-image-id\n' ;;
      *kite.version*) printf 'v0.0.1\n' ;;
      *) exit 99 ;;
    esac ;;
  image)
    case "$4" in
      '{{.Digest}}')
        if [[ "$5" == old-image-id ]]; then printf '%s\n' "${FAKE_OLD}"
        else printf '%s\n' "${FAKE_INSPECT_DIGEST:-${5##*@}}"; fi ;;
      *image.version*) printf '%s\n' "${FAKE_IMAGE_VERSION:-1.2.3}" ;;
      *image.revision*) printf '%s\n' "${FAKE_IMAGE_COMMIT:-${FAKE_COMMIT}}" ;;
      *) exit 99 ;;
    esac ;;
  login) cat >"${STATE_DIR}/login-token" ;;
  pull) exit 0 ;;
  rm) rm -f "${STATE_DIR}/current" ;;
  run)
    if [[ " $* " == *' --entrypoint /usr/local/bin/kite-probe '* ]]; then
      current="$(cat "${STATE_DIR}/current")"
      [[ "${FAKE_HEALTH_FAIL:-}" != 1 || "${current}" == *"${FAKE_OLD}" ]] || exit 1
      exit 0
    fi
    image="${@: -1}"
    [[ "${image}" != old-image-id ]] || image="ghcr.io/vod-studio/kite-server@${FAKE_OLD}"
    [[ "${FAKE_RUN_FAIL:-}" != 1 || "${image}" == *"${FAKE_OLD}" ]] || exit 125
    printf '%s\n' "${image}" >"${STATE_DIR}/current"
    if [[ "${FAKE_SIGNAL:-}" == 1 && "${image}" != *"${FAKE_OLD}" ]]; then kill -TERM "${PPID}"; fi
    ;;
  *) exit 99 ;;
esac
SHIM
cat >"${tmp}/bin/curl" <<'SHIM'
#!/usr/bin/env bash
printf 'curl %s\n' "$*" >>"${CALLS}"
[[ "${FAKE_API_FAIL:-}" != 1 ]] || exit 22
case "${@: -1}" in
  */releases/tags/v1.2.3)
    printf '{"draft":%s,"prerelease":false,"published_at":"2026-01-01","tag_name":"v1.2.3","target_commitish":"main"}\n' "${FAKE_DRAFT:-false}" ;;
  */commits/v1.2.3) printf '{"sha":"%s"}\n' "${FAKE_COMMIT}" ;;
  *) exit 99 ;;
esac
SHIM
cat >"${tmp}/bin/skopeo" <<'SHIM'
#!/usr/bin/env bash
printf 'skopeo %s\n' "$*" >>"${CALLS}"
cat "${FAKE_MANIFEST}"
SHIM
cat >"${tmp}/bin/cosign" <<'SHIM'
#!/usr/bin/env bash
printf 'cosign %s\n' "$*" >>"${CALLS}"
[[ "${FAKE_SIGNATURE_FAIL:-}" != 1 ]] || exit 1
[[ " $* " == *' --certificate-oidc-issuer https://token.actions.githubusercontent.com '* ]] || exit 99
[[ " $* " == *' --certificate-identity-regexp '* && "${@: -1}" == *"${FAKE_INDEX}" ]] || exit 99
printf '[{"critical":{"image":{"docker-manifest-digest":"%s"}}}]\n' "${FAKE_INDEX}"
SHIM
cat >"${tmp}/bin/sudo" <<'SHIM'
#!/usr/bin/env bash
printf 'sudo %s\n' "$*" >>"${CALLS}"
SHIM
chmod +x "${tmp}/bin/"*
fail=0
ref=ghcr.io/vod-studio/kite-server:v1.2.3
old="ghcr.io/vod-studio/kite-server@${FAKE_OLD}"
new="ghcr.io/vod-studio/kite-server@${FAKE_PLATFORM}"
reset_case() {
  rm -rf "${STATE_DIR}"; mkdir -m 0700 "${STATE_DIR}"
  printf '%s\n' "${old}" >"${STATE_DIR}/current"
  : >"${CALLS}"
  unset FAKE_SIGNATURE_FAIL FAKE_API_FAIL FAKE_DRAFT FAKE_IMAGE_VERSION FAKE_IMAGE_COMMIT FAKE_INSPECT_DIGEST \
    FAKE_SECRET_FAIL FAKE_DNS FAKE_RUN_FAIL FAKE_HEALTH_FAIL FAKE_SIGNAL FAKE_ARCH
}
deploy() {
  printf '%s\n' "${1:-}" | "${tmp}/installed/deploy-server.sh" "${ref}" >"${tmp}/output" 2>&1
  rc=$?
}
check() {
  local name="$1"; shift
  if "$@"; then printf 'ok %s\n' "${name}"; else printf 'FAIL %s\n' "${name}"; cat "${tmp}/output"; fail=1; fi
}
untouched() { [[ "$(cat "${STATE_DIR}/current")" == "${old}" ]] && ! grep -q '^podman rm ' "${CALLS}"; }
no_success() { [[ ! -e "${STATE_DIR}/deploy-history.json" ]] && ! grep -q '^Deployed ' "${tmp}/output"; }

reset_case; deploy
check 'approved platform digest is the running image' test "${rc}:$(cat "${STATE_DIR}/current")" = "0:${new}"
jq -e \
  --arg digest "${FAKE_INDEX}" --arg platform "${FAKE_PLATFORM}" --arg commit "${FAKE_COMMIT}" \
  --arg deployer "$(cat "${here}/deploy-version")" --arg build "$(cat "${tmp}/installed/deploy-build")" \
  'length == 1 and .[0].digest == $digest and .[0].platform_digest == $platform and .[0].commit == $commit and .[0].version == "v1.2.3" and .[0].deployer == $deployer and .[0].deployer_build == $build' "${STATE_DIR}/deploy-history.json" >/dev/null
check 'history records immutable index, platform, commit and installer version' test "$?" = 0
# Re-deploy recorded version with release/tag APIs and signing unavailable: the recorded digest is authoritative.
: >"${CALLS}"; export FAKE_API_FAIL=1 FAKE_SIGNATURE_FAIL=1; deploy
check 'recorded version rolls back by immutable digest without mutable release/tag lookup' test "${rc}" = 0
grep -Eq "^(curl|cosign) |^skopeo .*:v1.2.3$" "${CALLS}"
check 'rollback never resolves the mutable tag' test "$?" = 1
check 'recorded immutable platform is pulled' grep -q "^podman pull .*@${FAKE_PLATFORM}$" "${CALLS}"
reset_case; export FAKE_ARCH=aarch64; deploy
check 'arm64 deployment selects its signed index member' test "${rc}:$(cat "${STATE_DIR}/current")" = "0:ghcr.io/vod-studio/kite-server@${FAKE_ARM_PLATFORM}"
jq -e --arg platform "${FAKE_ARM_PLATFORM}" '.[0].platform_digest == $platform' "${STATE_DIR}/deploy-history.json" >/dev/null
check 'arm64 history records the platform rather than the index' test "$?" = 0

for failure in FAKE_SIGNATURE_FAIL FAKE_API_FAIL FAKE_SECRET_FAIL; do
  reset_case; export "${failure}=1"; deploy
  check "${failure}: fail closed" test "${rc}" != 0
  untouched
  check "${failure}: old container never removed" test "$?" = 0
  no_success
  check "${failure}: no success history" test "$?" = 0
done
for setting in FAKE_DRAFT=true FAKE_IMAGE_VERSION=9.9.9 FAKE_IMAGE_COMMIT=bad FAKE_INSPECT_DIGEST="${FAKE_OLD}" FAKE_INSPECT_DIGEST="${FAKE_INDEX}" FAKE_DNS=false FAKE_ARCH=riscv64; do
  reset_case; export "${setting%%=*}=${setting#*=}"; deploy
  check "${setting}: approval/metadata/preflight rejection" test "${rc}" != 0
  untouched
  check "${setting}: old instance unchanged" test "$?" = 0
done
for failure in FAKE_RUN_FAIL FAKE_HEALTH_FAIL FAKE_SIGNAL; do
  reset_case; export "${failure}=1"; deploy
  check "${failure}: transaction fails" test "${rc}" != 0
  check "${failure}: original actual digest restored" test "$(cat "${STATE_DIR}/current")" = "${old}"
  no_success
  check "${failure}: no success history" test "$?" = 0
done
reset_case
jq -n --arg digest "${FAKE_OLD}" --arg commit "${FAKE_COMMIT}" \
  '[{version:"v0.0.1",digest:$digest,commit:$commit,deployer:"0.9.0"}]' >"${STATE_DIR}/deploy-history.json"
chmod 0600 "${STATE_DIR}/deploy-history.json"
cp "${STATE_DIR}/deploy-history.json" "${tmp}/before-history.json"
export FAKE_RUN_FAIL=1; deploy
check 'failed candidate preserves existing trusted history byte-for-byte' cmp -s "${tmp}/before-history.json" "${STATE_DIR}/deploy-history.json"
grep -q '^Deployed ' "${tmp}/output"
check 'failed candidate with existing history does not report success' test "$?" = 1
reset_case
printf '[]\n' >"${STATE_DIR}/deploy-history.json"; chmod 0666 "${STATE_DIR}/deploy-history.json"; deploy
check 'writable history is not trusted' test "${rc}" != 0
untouched
check 'writable history rejection preserves running service' test "$?" = 0
reset_case
chmod 0777 "${tmp}/installed/kite-probe"; deploy
check 'untrusted probe permissions reject before replacement' test "${rc}" != 0
untouched
check 'untrusted probe leaves old instance intact' test "$?" = 0
chmod 0755 "${tmp}/installed/kite-probe"
reset_case
ref=ghcr.io/vod-studio/kite-server:latest; deploy
check 'direct sudo entry rejects non-version references' test "${rc}" = 2
check 'invalid direct reference does not invoke external deployment tools' test ! -s "${CALLS}"
ref=ghcr.io/vod-studio/kite-server:v1.2.3
reset_case; rm "${STATE_DIR}/current"; export FAKE_HEALTH_FAIL=1; deploy
check 'first deployment failure removes unhealthy instance' test ! -e "${STATE_DIR}/current"
check 'first deployment failure is not successful' test "${rc}" != 0
no_success
check 'first deployment failure records no success' test "$?" = 0
reset_case
exec 8>"${STATE_DIR}/deploy.lock"; flock -n 8; deploy
check 'host lock contention fails without touching old instance' test "${rc}" != 0
untouched
check 'lock spans transaction entry' test "$?" = 0
flock -u 8; exec 8>&-
reset_case; chmod 0777 "${STATE_DIR}"; deploy
check 'untrusted state permissions fail before mutation' test "${rc}" != 0
untouched
check 'bad state permissions preserve old container' test "$?" = 0
reset_case; deploy 'test-registry-token'
check 'registry token travels only over stdin' test "$(cat "${STATE_DIR}/login-token")" = test-registry-token
grep -q test-registry-token "${CALLS}" "${tmp}/output"
check 'registry token is absent from output and commands' test "$?" = 1

for bad in '' bash "${ref} && id" ghcr.io/other/image:v1.0.0 ghcr.io/vod-studio/kite-server:latest; do
  : >"${CALLS}"
  SSH_ORIGINAL_COMMAND="${bad}" "${here}/deploy-ssh-gate.sh" >"${tmp}/output" 2>&1
  check "gate rejects ${bad}" test "$?" = 2
  check 'rejected gate never invokes sudo' test ! -s "${CALLS}"
done
SSH_ORIGINAL_COMMAND=version "${here}/deploy-ssh-gate.sh" >"${tmp}/output" 2>&1
check 'read-only gate version succeeds' test "$?" = 0
check 'version never invokes sudo' test ! -s "${CALLS}"
SSH_ORIGINAL_COMMAND="${ref}" DEPLOY_SERVER=/untrusted SUDO=/untrusted "${here}/deploy-ssh-gate.sh" >"${tmp}/output" 2>&1
check 'gate uses only fixed root-managed command despite environment overrides' grep -qx "sudo -n /usr/local/libexec/kite/deploy-server.sh ${ref}" "${CALLS}"
exit "${fail}"
