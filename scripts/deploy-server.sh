#!/usr/bin/env bash
# 在生产服务器上部署一个 kite-server 版本(以 root 运行,由 deploy-ssh-gate.sh 经 sudo 调用)。
#
# 用法: deploy-server.sh ghcr.io/vod-studio/kite-server:v1.2.3
#   stdin:镜像仓库令牌(可为空;镜像公开时匿名拉取)。令牌只写进本次运行的临时 authfile,结束即删。
#
# 流程:拉取 → 锁定 digest → 替换容器 → 健康检查;失败时回滚到替换前的镜像并以非零退出。
# 容器只加入 kite 专用网络(默认 kite_network,别名 kite;不存在时创建),不映射宿主机端口:
# 调用方把自己的后端容器加入该网络,经 http://kite:3721 访问,彼此的其它容器互不可见。
# 访问令牌来自 podman secret(安装脚本 deploy-server-install.sh 创建),以文件形式挂进容器。
#
# 环境变量(均有默认值,测试时覆盖):PODMAN CURL CONTAINER NETWORK ALIAS SECRET STATE_DIR
#   HEALTH_TRIES HEALTH_SLEEP
set -euo pipefail

PODMAN="${PODMAN:-podman}"
CURL="${CURL:-curl}"
CONTAINER="${CONTAINER:-kite}"
NETWORK="${NETWORK:-kite_network}"
ALIAS="${ALIAS:-kite}"
SECRET="${SECRET:-kite-auth-token}"
STATE_DIR="${STATE_DIR:-/srv/kite}"
HEALTH_TRIES="${HEALTH_TRIES:-30}"
HEALTH_SLEEP="${HEALTH_SLEEP:-1}"

ref="${1:-}"
if ! [[ "${ref}" =~ ^ghcr\.io/vod-studio/kite-server:(v[0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  echo "拒绝:镜像必须形如 ghcr.io/vod-studio/kite-server:v1.2.3,收到: ${ref}" >&2
  exit 2
fi
version="${BASH_REMATCH[1]}"

work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT
authfile="${work}/auth.json"
echo '{}' >"${authfile}"

token=""
IFS= read -r token || true
if [ -n "${token}" ]; then
  printf '%s' "${token}" | "${PODMAN}" login --authfile "${authfile}" -u kite-deploy --password-stdin ghcr.io >/dev/null
fi

echo "拉取 ${ref}"
"${PODMAN}" pull --authfile "${authfile}" -q "${ref}" >/dev/null
digest="$("${PODMAN}" image inspect --format '{{.Digest}}' "${ref}")"
pinned="ghcr.io/vod-studio/kite-server@${digest}"

# 替换前正在运行的镜像(按 digest 记录),失败时回滚到它;首次部署时为空。
previous="$("${PODMAN}" inspect --format '{{.ImageName}}' "${CONTAINER}" 2>/dev/null || true)"
previous_version="$("${PODMAN}" inspect --format '{{index .Config.Labels "kite.version"}}' "${CONTAINER}" 2>/dev/null || true)"

run_container() { # image version
  "${PODMAN}" rm -f "${CONTAINER}" >/dev/null 2>&1 || true
  "${PODMAN}" run -d --name "${CONTAINER}" --restart=always \
    --network "${NETWORK}" --network-alias "${ALIAS}" \
    --secret "${SECRET},target=${SECRET},uid=65532,mode=0400" \
    -e "KITE_SERVER_AUTH_TOKEN_FILE=/run/secrets/${SECRET}" \
    --read-only --cap-drop=all --security-opt no-new-privileges \
    --label "kite.version=$2" \
    "$1" >/dev/null
}

healthy() {
  local ip i
  for ((i = 0; i < HEALTH_TRIES; i++)); do
    ip="$("${PODMAN}" inspect --format "{{(index .NetworkSettings.Networks \"${NETWORK}\").IPAddress}}" "${CONTAINER}" 2>/dev/null || true)"
    if [ -n "${ip}" ] && "${CURL}" -fsS --max-time 2 -o /dev/null "http://${ip}:3721/healthz"; then
      return 0
    fi
    sleep "${HEALTH_SLEEP}"
  done
  return 1
}

# 网络由 kite 拥有:首次部署时创建,已存在则不动。放在替换容器之前,创建失败时旧容器仍在运行。
"${PODMAN}" network create --ignore "${NETWORK}" >/dev/null

echo "启动 ${version} (${digest})"
# 启动失败(网络 / secret 缺失等)与健康检查失败同样走回滚:此时旧容器已被删除,不能让 set -e 直接退出。
if run_container "${pinned}" "${version}" && healthy; then
  mkdir -p "${STATE_DIR}"
  printf '%s %s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${version}" "${pinned}" >>"${STATE_DIR}/deploy-history"
  echo "部署成功: ${version}"
  exit 0
fi

echo "启动或健康检查失败,容器日志(最后 50 行):" >&2
"${PODMAN}" logs --tail 50 "${CONTAINER}" >&2 || true
if [ -z "${previous}" ]; then
  echo "首次部署失败,没有可回滚的版本" >&2
  "${PODMAN}" rm -f "${CONTAINER}" >/dev/null 2>&1 || true
  exit 1
fi
echo "回滚到 ${previous}" >&2
if run_container "${previous}" "${previous_version:-unknown}" && healthy; then
  echo "已回滚,${version} 未部署" >&2
else
  echo "回滚后健康检查仍失败,需要人工处理" >&2
fi
exit 1
