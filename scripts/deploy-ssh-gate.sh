#!/usr/bin/env bash
# 部署用户的 SSH 强制命令(authorized_keys 的 command=),不给 shell。
#
# 客户端发来的命令(SSH_ORIGINAL_COMMAND)只能是一个镜像引用,校验后经 sudo 交给 deploy-server.sh;
# stdin(镜像仓库令牌)原样透传。部署私钥即使泄露,能做的也只有「部署某个已发布的 kite 版本」。
set -euo pipefail

DEPLOY_SERVER="${DEPLOY_SERVER:-/usr/local/libexec/kite/deploy-server.sh}"
SUDO="${SUDO:-sudo}"

ref="${SSH_ORIGINAL_COMMAND:-}"
if ! [[ "${ref}" =~ ^ghcr\.io/vod-studio/kite-server:v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "拒绝:只接受 ghcr.io/vod-studio/kite-server:v1.2.3 形式的镜像引用" >&2
  exit 2
fi
exec "${SUDO}" -n "${DEPLOY_SERVER}" "${ref}"
