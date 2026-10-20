#!/usr/bin/env bash
# Administrator-only: verify the signed release bundle before executing this file.
# sudo ./deploy-server-install.sh /absolute/path/to/deployment-key.pub
set -euo pipefail
umask 077
[[ "$(id -u)" == 0 ]] || { echo 'Administrator/root is required' >&2; exit 2; }
[[ $# == 1 ]] || { echo 'Usage: deploy-server-install.sh <deployment-public-key>' >&2; exit 2; }
for tool in podman skopeo cosign curl jq flock sha256sum stat readlink install cmp mv ln \
  useradd getent ssh-keygen visudo systemctl openssl uname; do
  command -v "${tool}" >/dev/null || { echo "Missing dependency: ${tool}" >&2; exit 1; }
done
# Prefix is for isolated installation tests; no workflow or SSH gate can invoke this installer.
prefix="${KITE_INSTALL_ROOT:-}"
[[ -z "${prefix}" || "${prefix}" == /* ]] || { echo 'Installation root must be absolute' >&2; exit 2; }
readonly user_name=kite-deploy
libexec="${prefix}/usr/local/libexec/kite"
state="${prefix}/srv/kite"
home="${prefix}/var/lib/${user_name}"
here="$(cd "$(dirname "$0")" && pwd)"
files=(deploy-server.sh deploy-ssh-gate.sh deploy-server-install.sh deploy-version)
for file in "${files[@]}"; do
  [[ -f "${here}/${file}" && ! -L "${here}/${file}" ]] || { echo "Invalid package member: ${file}" >&2; exit 1; }
done
case "$(uname -m)" in
  x86_64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo 'Only Linux amd64/arm64 deployment hosts are supported' >&2; exit 1 ;;
esac
[[ "$(uname -s)" == Linux ]] || { echo 'Linux deployment host required' >&2; exit 1; }
for member in linux/amd64/kite-probe linux/arm64/kite-probe; do
  [[ -f "${here}/${member}" && ! -L "${here}/${member}" ]] || { echo "Invalid package member: ${member}" >&2; exit 1; }
done
probe_source="${here}/linux/${arch}/kite-probe"
# Hash canonical member names and bytes, not archive timestamps or extraction paths.
build_hash="$(
  for member in "${files[@]}" linux/amd64/kite-probe linux/arm64/kite-probe; do
    checksum="$(sha256sum <"${here}/${member}")"
    printf '%s\n%s\n' "${member}" "${checksum}"
  done | sha256sum
)"
build_hash="${build_hash%% *}"
files+=(kite-probe)
source_file() {
  if [[ "$1" == kite-probe ]]; then printf '%s\n' "${probe_source}"; else printf '%s/%s\n' "${here}" "$1"; fi
}
version="$(cat "${here}/deploy-version")"
[[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Invalid package version' >&2; exit 1; }
build="${version}-${build_hash}"
ssh-keygen -l -f "$1" >/dev/null
pubkey=''
IFS= read -r pubkey <"$1" || [[ -n "${pubkey}" ]]
[[ "${pubkey}" =~ ^ssh-(ed25519|rsa)\ [A-Za-z0-9+/=]+(\ .*)?$ && "${pubkey}" != *$'\r'* ]] || {
  echo 'Expected one plain ed25519 or RSA public key, without authorized_keys options' >&2; exit 2;
}
podman info >/dev/null
# Fail before changing authorization or binaries if boot recovery cannot be enabled.
systemctl enable podman-restart.service >/dev/null
systemctl is-enabled --quiet podman-restart.service

safe_directory() {
  local path="$1" mode="$2" cursor="$1" permissions
  # Reject symlinks or untrusted parents instead of following them as root.
  while [[ "${cursor}" != / && -n "${cursor}" ]]; do
    if [[ -e "${cursor}" || -L "${cursor}" ]]; then
      [[ -d "${cursor}" && ! -L "${cursor}" && "$(stat -c %u "${cursor}")" == 0 ]] || {
        echo "Untrusted installation directory: ${cursor}" >&2; return 1;
      }
      permissions="$(stat -c %a "${cursor}")"
      (( (8#${permissions} & 8#022) == 0 )) || {
        # /tmp is permitted only as the sticky ancestor of a test prefix.
        [[ -n "${prefix}" && "${cursor}" == /tmp && "${permissions}" == 1777 ]] || {
          echo "Writable installation directory: ${cursor}" >&2; return 1;
        }
      }
    fi
    cursor="$(dirname "${cursor}")"
  done
  install -d -m "${mode}" -o root -g root "${path}"
}
safe_directory "${state}" 0700
exec 9>"${state}/deploy.lock"
flock -n 9 || { echo 'Another host deployment or installer holds the lock' >&2; exit 1; }
safe_directory "${libexec}/versions" 0755
safe_directory "${prefix}/etc/sudoers.d" 0755
work="$(mktemp -d "${libexec}/.install.XXXXXXXX")"
trap 'rm -rf "${work}"' EXIT

if ! id "${user_name}" >/dev/null 2>&1; then
  useradd --system --create-home --home-dir "${home}" --shell /bin/bash "${user_name}"
fi
[[ "$(getent passwd "${user_name}" | cut -d: -f6)" == "${home}" ]] || {
  echo 'Existing deployment user has an unexpected home directory' >&2; exit 1;
}
# Migrate the old user-owned authorized_keys directory to root ownership.
[[ ! -L "${home}" && ! -L "${home}/.ssh" ]] || { echo 'Unsafe SSH home symlink' >&2; exit 1; }
chown root:root "${home}"
chmod 0755 "${home}"
if [[ -d "${home}/.ssh" ]]; then chown root:root "${home}/.ssh"; chmod 0755 "${home}/.ssh"; fi
safe_directory "${home}/.ssh" 0755

if ! podman secret exists kite-auth-token; then
  [[ ! -e "${state}/auth-token" && ! -L "${state}/auth-token" ]] || {
    echo 'Token backup exists but podman secret is missing; administrator must restore it explicitly' >&2; exit 1;
  }
  openssl rand -hex 32 >"${work}/auth-token"
  install -m 0600 -o root -g root "${work}/auth-token" "${state}/auth-token"
  podman secret create kite-auth-token "${state}/auth-token" >/dev/null
fi
podman secret inspect kite-auth-token >/dev/null
podman network create --ignore kite_network >/dev/null
podman network inspect kite_network | jq -e 'length == 1 and .[0].dns_enabled == true' >/dev/null

# Content-addressed builds are immutable. An administrator may explicitly install a new
# verified build of the same capability version without overwriting any prior payload.
target="${libexec}/versions/${build}"
if [[ -e "${target}" || -L "${target}" ]]; then
  [[ -d "${target}" && ! -L "${target}" && "$(stat -c '%u:%a' "${target}")" == 0:755 ]] || {
    echo 'Untrusted installed version directory' >&2; exit 1;
  }
  for file in "${files[@]}"; do
    mode=755; [[ "${file}" != deploy-version ]] || mode=644
    if ! [[ -f "${target}/${file}" && ! -L "${target}/${file}" && "$(stat -c '%u:%a' "${target}/${file}")" == "0:${mode}" ]] ||
      ! cmp -s "$(source_file "${file}")" "${target}/${file}"; then
      echo "Installed build ${build} has different bytes or permissions: ${file}" >&2
      exit 1
    fi
  done
  [[ -f "${target}/deploy-build" && ! -L "${target}/deploy-build" &&
    "$(stat -c '%u:%a' "${target}/deploy-build")" == 0:644 &&
    "$(cat "${target}/deploy-build")" == "${build_hash}" ]] || {
    echo 'Untrusted installed build identity' >&2; exit 1;
  }
else
  install -d -m 0755 "${work}/version"
  for file in "${files[@]}"; do
    mode=0755; [[ "${file}" != deploy-version ]] || mode=0644
    install -m "${mode}" -o root -g root "$(source_file "${file}")" "${work}/version/${file}"
  done
  printf '%s\n' "${build_hash}" >"${work}/version/deploy-build"
  chmod 0644 "${work}/version/deploy-build"
  mv "${work}/version" "${target}"
fi
printf 'Defaults:%s env_reset, !setenv, secure_path=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n%s ALL=(root) NOPASSWD: NOSETENV: /usr/local/libexec/kite/deploy-server.sh\n' \
  "${user_name}" "${user_name}" >"${work}/sudoers"
visudo -cqf "${work}/sudoers" >/dev/null
chmod 0440 "${work}/sudoers"
mv -Tf "${work}/sudoers" "${prefix}/etc/sudoers.d/${user_name}"
# Atomic current switch; the forced command and sudo entry always resolve root-owned versioned files.
ln -s "versions/${build}" "${work}/current"
mv -Tf "${work}/current" "${libexec}/current"
for file in deploy-server.sh deploy-ssh-gate.sh deploy-version; do
  ln -s "current/${file}" "${work}/${file}"
  mv -Tf "${work}/${file}" "${libexec}/${file}"
done
printf 'restrict,command="/usr/local/libexec/kite/deploy-ssh-gate.sh" %s\n' "${pubkey}" >"${work}/authorized_keys"
chmod 0644 "${work}/authorized_keys"
mv -Tf "${work}/authorized_keys" "${home}/.ssh/authorized_keys"
printf 'Installed kite-deployer %s build=%s; deployment key cannot update root-managed files\n' "${version}" "${build_hash}"
