#!/usr/bin/env bash
# Installed root-owned forced command. No client-controlled paths or trust settings.
set -euo pipefail
ref="${SSH_ORIGINAL_COMMAND:-}"
if [[ "${ref}" == version ]]; then
  here="$(cd "$(dirname "$(readlink -f "$0")")" && pwd)"
  version="$(cat "${here}/deploy-version")"
  [[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Invalid installed deployer version' >&2; exit 1; }
  printf 'kite-deployer %s\n' "${version}"
  exit 0
fi
[[ "${ref}" =~ ^ghcr\.io/vod-studio/kite-server:v[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
  echo 'Only version or ghcr.io/vod-studio/kite-server:vX.Y.Z is accepted' >&2
  exit 2
}
exec sudo -n /usr/local/libexec/kite/deploy-server.sh "${ref}"
