#!/usr/bin/env bash
# Root-managed deployment transaction. Only the fixed repository's public releases are trusted.
# stdin may contain a GHCR pull token; it is never a trust-root or GitHub API credential.
set -euo pipefail
umask 077

PODMAN="${PODMAN:-podman}"
CURL="${CURL:-curl}"
COSIGN="${COSIGN:-cosign}"
STATE_DIR="${STATE_DIR:-/srv/kite}"
HEALTH_TRIES="${HEALTH_TRIES:-6}"
HEALTH_SLEEP="${HEALTH_SLEEP:-2}"
readonly repository=ghcr.io/vod-studio/kite-server
readonly network=kite_network container=kite secret=kite-auth-token
readonly issuer=https://token.actions.githubusercontent.com
readonly identity='^https://github\.com/VOD-Studio/kite/\.github/workflows/(release|release-please)\.yml@refs/heads/main$'
here="$(cd "$(dirname "$(readlink -f "$0")")" && pwd)"
deployer="$(cat "${here}/deploy-version")"
[[ "${deployer}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Invalid deployer version' >&2; exit 2; }
ref="${1:-}"
[[ $# == 1 && "${ref}" =~ ^ghcr\.io/vod-studio/kite-server:(v[0-9]+\.[0-9]+\.[0-9]+)$ ]] || {
  echo 'Only ghcr.io/vod-studio/kite-server:vX.Y.Z is accepted' >&2; exit 2;
}
version="${BASH_REMATCH[1]}"
SKOPEO="${SKOPEO:-skopeo}"
for tool in "${PODMAN}" "${CURL}" "${COSIGN}" "${SKOPEO}" jq flock stat mktemp sha256sum; do
  command -v "${tool}" >/dev/null || { echo "Missing dependency: ${tool}" >&2; exit 1; }
done
# The installer creates this root-only directory. Never adopt user-controlled state or locks.
[[ -d "${STATE_DIR}" && ! -L "${STATE_DIR}" && "$(stat -c '%u:%a' "${STATE_DIR}")" == 0:700 ]] || {
  echo 'Deployment state must be a root-owned mode-0700 directory' >&2; exit 1;
}
exec 9>"${STATE_DIR}/deploy.lock"
flock -n 9 || { echo 'Another host deployment or installer holds the lock' >&2; exit 1; }
probe="${here}/kite-probe"
[[ -f "${probe}" && ! -L "${probe}" && "$(stat -c '%u:%a' "${probe}")" == 0:755 ]] || {
  echo 'Missing or untrusted installed deployment probe' >&2; exit 1;
}
[[ -f "${here}/deploy-build" && ! -L "${here}/deploy-build" &&
  "$(stat -c '%u:%a' "${here}/deploy-build")" == 0:644 ]] || {
  echo 'Missing or untrusted installed build identity' >&2; exit 1;
}
deployer_build="$(cat "${here}/deploy-build")"
[[ "${deployer_build}" =~ ^[a-f0-9]{64}$ ]] || { echo 'Invalid installed build identity' >&2; exit 1; }
work="$(mktemp -d "${STATE_DIR}/.deploy.XXXXXXXX")"
previous='' previous_image='' previous_version='' previous_commit='' changed=0 committed=0

run_container() {
  "${PODMAN}" rm -f --ignore "${container}" >/dev/null 2>&1 || return 1
  "${PODMAN}" run -d --pull=never --name "${container}" --restart=always \
    --network "${network}" --network-alias kite \
    --user 65532:65532 --secret "${secret},target=${secret},uid=65532,mode=0400" \
    -e "KITE_SERVER_AUTH_TOKEN_FILE=/run/secrets/${secret}" \
    --read-only --cap-drop=all --security-opt no-new-privileges \
    --label "kite.version=$2" --label "kite.commit=$3" --label "kite.deployer=${deployer}" "$1" 9>&- >/dev/null
}
healthy() {
  local i
  for ((i = 0; i < HEALTH_TRIES; i++)); do
    if "${PODMAN}" run --rm --pull=never --network "${network}" --user 65532:65532 \
      --secret "${secret},target=${secret},uid=65532,mode=0400" \
      -e "KITE_SERVER_AUTH_TOKEN_FILE=/run/secrets/${secret}" \
      --read-only --cap-drop=all --security-opt no-new-privileges \
      --volume "${probe}:/usr/local/bin/kite-probe:ro,Z" \
      --entrypoint /usr/local/bin/kite-probe "${probe_image}" 9>&- >"${work}/probe.log" 2>&1; then
      cat "${work}/probe.log"
      return 0
    fi
    sleep "${HEALTH_SLEEP}"
  done
  cat "${work}/probe.log" >&2
  return 1
}
finish() {
  local status=$?
  trap - EXIT
  trap '' HUP INT TERM
  if ((changed && !committed)); then
    status=1
    if [[ -n "${previous}" ]]; then
      echo "Deployment failed; restoring ${previous}" >&2
      if run_container "${previous_image}" "${previous_version}" "${previous_commit}" && healthy; then
        echo 'Previous digest restored; requested deployment failed' >&2
      else
        echo 'CRITICAL: rollback failed; administrator intervention required' >&2
      fi
    else
      "${PODMAN}" rm -f "${container}" >/dev/null 2>&1 || true
      echo 'First deployment failed; no previous digest exists' >&2
    fi
  fi
  rm -rf "${work}"
  exit "${status}"
}
trap finish EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

"${PODMAN}" info >/dev/null
"${PODMAN}" secret inspect "${secret}" >/dev/null
"${PODMAN}" network create --ignore "${network}" >/dev/null
"${PODMAN}" network inspect "${network}" | jq -e 'length == 1 and .[0].dns_enabled == true' >/dev/null
# Rollback identity is read from the running container's actual image, never its mutable tag.
if "${PODMAN}" container exists "${container}"; then
  image_id="$("${PODMAN}" inspect --format '{{.Image}}' "${container}")"
  previous_image="${image_id}"
  previous_digest="$("${PODMAN}" image inspect --format '{{.Digest}}' "${image_id}")"
  [[ "${previous_digest}" =~ ^sha256:[a-f0-9]{64}$ ]] || { echo 'Cannot pin current image for rollback' >&2; exit 1; }
  previous="${repository}@${previous_digest}"
  previous_version="$("${PODMAN}" inspect --format '{{index .Config.Labels "kite.version"}}' "${container}")"
  previous_commit="$("${PODMAN}" image inspect --format '{{index .Labels "org.opencontainers.image.revision"}}' "${image_id}")"
fi

authfile="${work}/config.json"
printf '{}\n' >"${authfile}"
token=''
IFS= read -r token || true
if [[ -n "${token}" ]]; then
  printf '%s' "${token}" | "${PODMAN}" login --authfile "${authfile}" -u kite-deploy --password-stdin ghcr.io >/dev/null 2>&1
fi
unset token

history="${STATE_DIR}/deploy-history.json"
if [[ -e "${history}" ]]; then
  [[ -f "${history}" && ! -L "${history}" && "$(stat -c '%u:%a' "${history}")" == 0:600 ]] || {
    echo 'Untrusted deployment history permissions' >&2; exit 1;
  }
  jq -e 'type == "array"' "${history}" >/dev/null
else
  printf '[]\n' >"${work}/empty-history"
  history="${work}/empty-history"
fi
record="$(jq -c --arg version "${version}" '[.[] | select(.version == $version)] | last // empty' "${history}")"
if [[ -n "${record}" ]]; then
  digest="$(jq -er '.digest' <<<"${record}")"
  commit="$(jq -er '.commit' <<<"${record}")"
  [[ "${digest}" =~ ^sha256:[a-f0-9]{64}$ && "${commit}" =~ ^[a-f0-9]{40}$ ]] || { echo 'Invalid trusted history record' >&2; exit 1; }
  pinned="${repository}@${digest}"
  echo "Using previously deployed immutable image ${version} ${digest}"
  "${SKOPEO}" inspect --authfile "${authfile}" --raw "docker://${pinned}" >"${work}/manifest.json"
  read -r manifest_hash _ < <(sha256sum "${work}/manifest.json")
  [[ "sha256:${manifest_hash}" == "${digest}" ]] || { echo 'Recorded manifest digest mismatch' >&2; exit 1; }
else
  # GitHub public API: fail closed for drafts, prereleases, deleted releases or moved/invalid tags.
  api=https://api.github.com/repos/VOD-Studio/kite
  "${CURL}" --fail --silent --show-error --proto '=https' --max-time 30 \
    -H 'Accept: application/vnd.github+json' "${api}/releases/tags/${version}" >"${work}/release.json"
  jq -e --arg version "${version}" '.draft == false and .prerelease == false and .published_at != null and .tag_name == $version' "${work}/release.json" >/dev/null
  "${CURL}" --fail --silent --show-error --proto '=https' --max-time 30 \
    -H 'Accept: application/vnd.github+json' "${api}/commits/${version}" >"${work}/commit.json"
  commit="$(jq -er '.sha' "${work}/commit.json")"
  [[ "${commit}" =~ ^[a-f0-9]{40}$ ]] || { echo 'Invalid release commit' >&2; exit 1; }
  jq -e --arg commit "${commit}" '.target_commitish == "main" or .target_commitish == $commit' "${work}/release.json" >/dev/null
  "${SKOPEO}" inspect --authfile "${authfile}" --raw "docker://${ref}" >"${work}/manifest.json"
  read -r manifest_hash _ < <(sha256sum "${work}/manifest.json")
  digest="sha256:${manifest_hash}"
  [[ "${digest}" =~ ^sha256:[a-f0-9]{64}$ ]] || { echo 'Invalid image digest' >&2; exit 1; }
  pinned="${repository}@${digest}"
  # Cosign verifies immutable bytes and the Fulcio/Rekor keyless proof; no client-supplied identity flags.
  DOCKER_CONFIG="${work}" "${COSIGN}" verify \
    --certificate-identity-regexp "${identity}" --certificate-oidc-issuer "${issuer}" \
    "${pinned}" >"${work}/signature.json"
  jq -e --arg digest "${digest}" 'type == "array" and length > 0 and all(.[]; .critical.image["docker-manifest-digest"] == $digest)' "${work}/signature.json" >/dev/null
fi
# Podman can report the index digest for an index reference; select and pin its
# native platform member explicitly instead of mistaking the index for a platform.
case "$(uname -m)" in
  x86_64) architecture=amd64 ;;
  aarch64|arm64) architecture=arm64 ;;
  *) echo 'Unsupported deployment architecture' >&2; exit 1 ;;
esac
platform_digest="$(jq -er --arg architecture "${architecture}" '
  [.manifests[]? | select(.platform.os == "linux" and .platform.architecture == $architecture)]
  | if length == 1 then .[0].digest else error("Expected one native Linux platform in signed index") end
' "${work}/manifest.json")"
[[ "${platform_digest}" =~ ^sha256:[a-f0-9]{64}$ ]] || { echo 'Invalid platform manifest digest' >&2; exit 1; }
pinned="${repository}@${platform_digest}"
"${PODMAN}" pull --authfile "${authfile}" -q "${pinned}" >/dev/null
actual_digest="$("${PODMAN}" image inspect --format '{{.Digest}}' "${pinned}")"
image_version="$("${PODMAN}" image inspect --format '{{index .Labels "org.opencontainers.image.version"}}' "${pinned}")"
image_commit="$("${PODMAN}" image inspect --format '{{index .Labels "org.opencontainers.image.revision"}}' "${pinned}")"
[[ "${actual_digest}" == "${platform_digest}" && "${image_version}" == "${version#v}" && "${image_commit}" == "${commit}" ]] || {
  echo 'Image digest/version/revision does not match the approved release' >&2; exit 1;
}
# Mount the root-managed probe into the verified image, including releases predating the probe.
probe_image="${pinned}"
# Ensure history can be written before stopping a healthy service.
jq --arg time "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg version "${version}" \
  --arg digest "${digest}" --arg platform_digest "${actual_digest}" --arg commit "${commit}" \
  --arg deployer "${deployer}" --arg deployer_build "${deployer_build}" \
  '. + [{time: $time, version: $version, commit: $commit, digest: $digest, platform_digest: $platform_digest, deployer: $deployer, deployer_build: $deployer_build}]' \
  "${history}" >"${work}/history.json"
changed=1
run_container "${pinned}" "${version}" "${commit}" || exit 1
healthy || exit 1
# Commit point: a signal before this block rolls back; afterward the healthy release is committed.
trap '' HUP INT TERM
mv "${work}/history.json" "${STATE_DIR}/deploy-history.json"
committed=1
printf 'Deployed version=%s commit=%s digest=%s deployer=%s build=%s\n' "${version}" "${commit}" "${digest}" "${deployer}" "${deployer_build}"
