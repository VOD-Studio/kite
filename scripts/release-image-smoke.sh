#!/usr/bin/env bash
# 使用 GoReleaser snapshot 的真实发布二进制构建 Dockerfile.release，并验证运行行为。
# 用法: release-image-smoke.sh [dist目录] [镜像名]
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
dist=${1:-"$root/dist"}
image=${2:-kite-server:release-ci}
arch=$(docker info --format '{{.Architecture}}')
case "$arch" in
  amd64|x86_64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "unsupported Docker architecture: $arch" >&2; exit 1 ;;
esac
# GoReleaser 的 amd64 目录带微架构后缀；必须唯一匹配，防止混用陈旧产物。
shopt -s nullglob
servers=("$dist"/server_linux_"$arch"*/server)
probes=("$dist"/deploy-probe_linux_"$arch"*/kite-probe)
if [ "${#servers[@]}" -ne 1 ] || [ "${#probes[@]}" -ne 1 ]; then
  echo "expected one server and deploy-probe snapshot binary for linux/$arch in $dist" >&2
  exit 1
fi
context=$(mktemp -d)
trap 'rm -rf "$context"' EXIT
mkdir -p "$context/linux/$arch"
cp "${servers[0]}" "$context/linux/$arch/server"
cp "${probes[0]}" "$context/linux/$arch/kite-probe"
docker build --platform "linux/$arch" --build-arg "TARGETPLATFORM=linux/$arch" \
  -f "$root/Dockerfile.release" -t "$image" "$context"
bash "$root/scripts/docker-smoke.sh" "$image"
