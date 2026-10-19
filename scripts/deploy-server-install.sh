#!/usr/bin/env bash
# 在生产服务器上一次性安装 kite 部署入口(以 root 运行,可重复执行;改了部署脚本后重跑即更新)。
#
# 用法: deploy-server-install.sh <部署公钥文件>
#
# 做的事:
#   1. 建系统用户 kite-deploy(无密码),authorized_keys 只允许执行 deploy-ssh-gate.sh(restrict:无 shell、无转发);
#   2. 安装 deploy-ssh-gate.sh / deploy-server.sh 到 /usr/local/libexec/kite/(root 所有);
#   3. sudoers 只放行 kite-deploy 以 root 执行 deploy-server.sh 这一个命令;
#   4. 没有 podman secret kite-auth-token 时生成访问令牌(32 字节随机数),副本存 /srv/kite/auth-token(0600),
#      供同机的调用方(violet)配置;已有则保留,不轮换。
set -euo pipefail

USER_NAME="kite-deploy"
LIBEXEC="/usr/local/libexec/kite"
STATE_DIR="/srv/kite"
SECRET="kite-auth-token"
here="$(cd "$(dirname "$0")" && pwd)"

pubkey_file="${1:?用法: $0 <部署公钥文件>}"
pubkey="$(head -n1 "${pubkey_file}")"
[[ "${pubkey}" =~ ^ssh-(ed25519|rsa)\  ]] || { echo "不是 ssh 公钥: ${pubkey_file}" >&2; exit 2; }
[ "$(id -u)" -eq 0 ] || { echo "需要 root" >&2; exit 2; }

if ! id "${USER_NAME}" >/dev/null 2>&1; then
  useradd --system --create-home --home-dir "/var/lib/${USER_NAME}" --shell /bin/bash "${USER_NAME}"
fi

install -d -m 0755 -o root -g root "${LIBEXEC}"
install -m 0755 -o root -g root "${here}/deploy-ssh-gate.sh" "${LIBEXEC}/deploy-ssh-gate.sh"
install -m 0755 -o root -g root "${here}/deploy-server.sh" "${LIBEXEC}/deploy-server.sh"

sudoers="$(mktemp)"
printf '%s ALL=(root) NOPASSWD: %s\n' "${USER_NAME}" "${LIBEXEC}/deploy-server.sh" >"${sudoers}"
visudo -cqf "${sudoers}"
install -m 0440 -o root -g root "${sudoers}" "/etc/sudoers.d/${USER_NAME}"
rm -f "${sudoers}"

home="$(getent passwd "${USER_NAME}" | cut -d: -f6)"
install -d -m 0700 -o "${USER_NAME}" -g "${USER_NAME}" "${home}/.ssh"
printf 'restrict,command="%s" %s\n' "${LIBEXEC}/deploy-ssh-gate.sh" "${pubkey}" >"${home}/.ssh/authorized_keys"
chown "${USER_NAME}:${USER_NAME}" "${home}/.ssh/authorized_keys"
chmod 0600 "${home}/.ssh/authorized_keys"

install -d -m 0700 -o root -g root "${STATE_DIR}"
if ! podman secret exists "${SECRET}"; then
  (umask 077 && openssl rand -hex 32 >"${STATE_DIR}/auth-token")
  podman secret create "${SECRET}" "${STATE_DIR}/auth-token" >/dev/null
  echo "已生成访问令牌:${STATE_DIR}/auth-token"
fi
echo "安装完成:${USER_NAME} 只能执行 ${LIBEXEC}/deploy-ssh-gate.sh"
