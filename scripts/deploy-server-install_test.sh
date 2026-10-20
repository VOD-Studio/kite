#!/usr/bin/env bash
# 隔离 Linux 安装测试:替换特权服务,检查真实文件权限、链接和幂等行为。
set -uo pipefail
if [[ "$(uname -s)" != Linux ]]; then
  echo 'skip: 部署安装测试需要 Linux;CI 的 Hygiene job 执行完整测试'
  exit 0
fi
here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT
mkdir -p "${tmp}/bin" "${tmp}/package"
cp "${here}/deploy-server.sh" "${here}/deploy-ssh-gate.sh" "${here}/deploy-server-install.sh" "${here}/deploy-version" "${tmp}/package/"
mkdir -p "${tmp}/package/linux/amd64" "${tmp}/package/linux/arm64"
for arch in amd64 arm64; do
  printf '#!/bin/sh\nexit 0\n' >"${tmp}/package/linux/${arch}/kite-probe"
  chmod 0755 "${tmp}/package/linux/${arch}/kite-probe"
done
REAL_STAT="$(command -v stat)"
REAL_INSTALL="$(command -v install)"
export REAL_STAT REAL_INSTALL
export KITE_INSTALL_ROOT="${tmp}/root" CALLS="${tmp}/calls"
export PATH="${tmp}/bin:${PATH}"
cat >"${tmp}/bin/id" <<'SHIM'
#!/usr/bin/env bash
if [[ "$1" == -u ]]; then printf '%s\n' "${FAKE_UID:-0}"; else [[ -d "${KITE_INSTALL_ROOT}/var/lib/kite-deploy" ]]; fi
SHIM
cat >"${tmp}/bin/stat" <<'SHIM'
#!/usr/bin/env bash
case "$2" in
  %u) printf '0\n' ;;
  %u:%a) printf '0:%s\n' "$("${REAL_STAT}" -c %a "$3")" ;;
  *) exec "${REAL_STAT}" "$@" ;;
esac
SHIM
cat >"${tmp}/bin/install" <<'SHIM'
#!/usr/bin/env bash
args=()
while (($#)); do
  case "$1" in -o|-g) shift 2 ;; *) args+=("$1"); shift ;; esac
done
exec "${REAL_INSTALL}" "${args[@]}"
SHIM
cat >"${tmp}/bin/useradd" <<'SHIM'
#!/usr/bin/env bash
mkdir -p "${KITE_INSTALL_ROOT}/var/lib/kite-deploy"
SHIM
cat >"${tmp}/bin/getent" <<'SHIM'
#!/usr/bin/env bash
printf 'kite-deploy:x:1001:1001::%s/var/lib/kite-deploy:/bin/bash\n' "${KITE_INSTALL_ROOT}"
SHIM
cat >"${tmp}/bin/systemctl" <<'SHIM'
#!/usr/bin/env bash
printf 'systemctl %s\n' "$*" >>"${CALLS}"
[[ "${FAKE_BOOT_FAIL:-}" != 1 ]]
SHIM
cat >"${tmp}/bin/visudo" <<'SHIM'
#!/usr/bin/env bash
[[ "${FAKE_SUDOERS_FAIL:-}" != 1 ]]
SHIM
cat >"${tmp}/bin/podman" <<'SHIM'
#!/usr/bin/env bash
printf 'podman %s\n' "$*" >>"${CALLS}"
case "$1 $2" in
  'secret exists') [[ -e "${KITE_INSTALL_ROOT}/secret-created" ]] ;;
  'secret create') touch "${KITE_INSTALL_ROOT}/secret-created" ;;
  'network inspect') printf '[{"dns_enabled":true}]\n' ;;
esac
SHIM
for tool in chown skopeo cosign; do
  printf '#!/usr/bin/env bash\nexit 0\n' >"${tmp}/bin/${tool}"
done
chmod +x "${tmp}/bin/"*
ssh-keygen -q -t ed25519 -N '' -f "${tmp}/key"
fail=0
run_install() {
  "${tmp}/package/deploy-server-install.sh" "${tmp}/key.pub" >"${tmp}/output" 2>&1
  rc=$?
}
check() {
  local name="$1"; shift
  if "$@"; then printf 'ok %s\n' "${name}"; else printf 'FAIL %s\n' "${name}"; cat "${tmp}/output"; fail=1; fi
}
version="$(cat "${here}/deploy-version")"
export FAKE_BOOT_FAIL=1
run_install
check 'boot recovery failure is fatal' test "${rc}" != 0
check 'boot failure creates no deployment authorization' test ! -e "${KITE_INSTALL_ROOT}/var/lib/kite-deploy/.ssh/authorized_keys"
unset FAKE_BOOT_FAIL
export FAKE_UID=1000
run_install
check 'non-administrator cannot install' test "${rc}" = 2
unset FAKE_UID
printf 'bad-version\n' >"${tmp}/package/deploy-version"
run_install
check 'invalid installer version fails' test "${rc}" != 0
cp "${here}/deploy-version" "${tmp}/package/deploy-version"
run_install
check 'valid installation completes' test "${rc}" = 0
libexec="${KITE_INSTALL_ROOT}/usr/local/libexec/kite"
initial_build="$(cat "${libexec}/current/deploy-build")"
initial_current="versions/${version}-${initial_build}"
check 'current names capability version and immutable content hash' test "$(readlink "${libexec}/current")" = "${initial_current}"
check 'forced gate is executable' test -x "${libexec}/deploy-ssh-gate.sh"
check 'state is private' test "$("${REAL_STAT}" -c %a "${KITE_INSTALL_ROOT}/srv/kite")" = 700
check 'sudoers is read-only root permission mode' test "$("${REAL_STAT}" -c %a "${KITE_INSTALL_ROOT}/etc/sudoers.d/kite-deploy")" = 440
check 'SSH home cannot be modified by deployment user' test "$("${REAL_STAT}" -c %a "${KITE_INSTALL_ROOT}/var/lib/kite-deploy")" = 755
check 'SSH authorization is read-only to deployment user' test "$("${REAL_STAT}" -c %a "${KITE_INSTALL_ROOT}/var/lib/kite-deploy/.ssh/authorized_keys")" = 644
check 'restart service must be enabled' grep -qx 'systemctl is-enabled --quiet podman-restart.service' "${CALLS}"
: >"${CALLS}"
run_install
check 'identical version installs idempotently' test "${rc}" = 0
grep -q '^podman secret create' "${CALLS}"
check 'idempotent installer never rotates existing secret' test "$?" = 1
SSH_ORIGINAL_COMMAND=version "${libexec}/deploy-ssh-gate.sh" >"${tmp}/output" 2>&1
check 'installed version gate resolves versioned payload' grep -qx "kite-deployer ${version}" "${tmp}/output"
chmod 0777 "${libexec}/${initial_current}/deploy-server.sh"
run_install
check 'tampered installed permissions fail instead of reporting success' test "${rc}" != 0
chmod 0755 "${libexec}/${initial_current}/deploy-server.sh"
for arch in amd64 arm64; do
  printf '\n# rebuilt probe\n' >>"${tmp}/package/linux/${arch}/kite-probe"
done
run_install
check 'administrator can install rebuilt probe at same capability version' test "${rc}" = 0
second_current="$(readlink "${libexec}/current")"
check 'rebuilt probe gets a distinct immutable directory' test "${second_current}" != "${initial_current}"
check 'prior build remains untouched' cmp -s "${here}/deploy-server.sh" "${libexec}/${initial_current}/deploy-server.sh"
SSH_ORIGINAL_COMMAND=version "${libexec}/deploy-ssh-gate.sh" >"${tmp}/output" 2>&1
check 'rebuilt probe does not raise CI capability version' grep -qx "kite-deployer ${version}" "${tmp}/output"
run_install
check 'identical rebuilt payload installs idempotently' test "${rc}" = 0
check 'idempotent rebuild retains identical directory' test "$(readlink "${libexec}/current")" = "${second_current}"
printf '999.0.0\n' >"${tmp}/package/deploy-version"
export FAKE_SUDOERS_FAIL=1
run_install
check 'sudoers validation failure is fatal' test "${rc}" != 0
check 'sudoers failure does not activate new version' test "$(readlink "${libexec}/current")" = "${second_current}"
unset FAKE_SUDOERS_FAIL
run_install
check 'new immutable version activates' test "${rc}" = 0
check 'upgrade retains old payload' test -x "${libexec}/${initial_current}/deploy-server.sh"
check 'upgrade switches current atomically' test "$(readlink "${libexec}/current")" = "versions/999.0.0-$(cat "${libexec}/current/deploy-build")"
exit "${fail}"
