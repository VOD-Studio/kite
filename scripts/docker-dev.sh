#!/usr/bin/env bash
# 本地开发工具链;不挂载 Docker socket、宿主机 HOME 或发布凭据。
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
command -v docker >/dev/null || { echo '需要安装并启动 Docker' >&2; exit 1; }
docker info >/dev/null

go_version="$(awk '$1 == "toolchain" { sub(/^go/, "", $2); print $2; exit }' "${root}/go.mod")"
if [[ -z "$go_version" ]]; then
  go_version="$(awk '$1 == "go" { print $2; exit }' "${root}/go.mod")"
fi
[[ "$go_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'go.mod 缺少有效的 Go 工具链版本' >&2; exit 1; }

uid="$(id -u)"
gid="$(id -g)"
image="kite-dev:${uid}-${gid}"
build_args=(--build-arg "GO_VERSION=${go_version}" --build-arg "LOCAL_UID=${uid}" --build-arg "LOCAL_GID=${gid}")
versions="$(make --no-print-directory -s -C "$root" ci-env)"
while IFS='=' read -r name value; do
  case "$name" in
    NODE_VERSION|GOLANGCI_LINT_VERSION|BUF_VERSION|PYYAML_VERSION) build_args+=(--build-arg "${name}=${value}") ;;
  esac
done <<< "$versions"
if [[ -n "${GOPROXY:-}" ]]; then build_args+=(--build-arg "GOPROXY=${GOPROXY}"); fi

docker build --file "${root}/Dockerfile.dev" --tag "$image" "${build_args[@]}" "$root"

# 每个工作区与 UID 有独立的 Linux 缓存,避免复用 macOS 的原生依赖。
cache_key="$(printf '%s' "${root}:${uid}:${gid}" | cksum)"
cache_key="kite-dev-${cache_key%% *}"
run_args=(--rm --init --workdir /workspace --mount "type=bind,source=${root},target=/workspace")
while IFS=' ' read -r suffix target; do
  run_args+=(--mount "type=volume,source=${cache_key}-${suffix},target=${target}")
done <<'VOLUMES'
go-mod /home/kite/go/pkg/mod
go-build /home/kite/.cache/go-build
npm /home/kite/.npm
node-modules /workspace/packages/kite-player/node_modules
playwright /home/kite/.cache/ms-playwright
rust-target /workspace/interop/rust/target
VOLUMES
if [[ -t 0 && -t 1 ]]; then run_args+=(-it --env "TERM=${TERM:-xterm-256color}"); fi
if [[ -n "${GOPROXY:-}" ]]; then run_args+=(--env "GOPROXY=${GOPROXY}"); fi
exec docker run "${run_args[@]}" "$image" bash -c "${KITE_DOCKER_COMMAND:-exec bash}"
