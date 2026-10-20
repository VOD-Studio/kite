#!/usr/bin/env bash
# scripts/deploy-server.sh 与 deploy-ssh-gate.sh 的单元测试:用假的 podman / curl / sudo 覆盖
# 成功、失败回滚、首次部署失败、非法引用、令牌处理,断言最终运行的镜像与调用记录。
# 运行: scripts/deploy-server_test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

# 假 podman:inspect 按 --format 返回预置值;run 把镜像(最后一个参数)记为当前运行的镜像。
cat >"${tmp}/podman" <<'SHIM'
#!/usr/bin/env bash
echo "$*" >>"${CALLS}"
case "$1" in
  login) cat >"${STATE}/login-token" ;;
  pull) ;;
  image) echo "sha256:${FAKE_DIGEST}" ;;
  inspect)
    case "$3" in
      *ImageName*) [ -n "${FAKE_PREVIOUS}" ] && echo "${FAKE_PREVIOUS}" || exit 125 ;;
      *Labels*) [ -n "${FAKE_PREVIOUS}" ] && echo "v0.0.1" || exit 125 ;;
      *IPAddress*) echo "10.0.0.5" ;;
    esac ;;
  run)
    if [ -n "${FAKE_RUN_FAIL:-}" ] && grep -Eq "${FAKE_RUN_FAIL}" <<<"${@: -1}"; then exit 125; fi
    echo "${@: -1}" >"${STATE}/current" ;;
  rm|logs|network) ;;
  *) echo "unexpected podman call: $*" >&2; exit 99 ;;
esac
SHIM
# 假 curl:当前运行的镜像匹配 FAKE_HEALTHY 时健康。
cat >"${tmp}/curl" <<'SHIM'
#!/usr/bin/env bash
grep -Eq "${FAKE_HEALTHY}" "${STATE}/current" 2>/dev/null
SHIM
cat >"${tmp}/sudo" <<'SHIM'
#!/usr/bin/env bash
echo "sudo $*" >>"${CALLS}"
SHIM
chmod +x "${tmp}/podman" "${tmp}/curl" "${tmp}/sudo"

fail=0
deploy() { # ref previous healthy_regex token
  rm -rf "${tmp}/state" && mkdir -p "${tmp}/state"
  : >"${tmp}/calls.log"
  printf '%s\n' "$4" | FAKE_RUN_FAIL="${FAKE_RUN_FAIL:-}" PODMAN="${tmp}/podman" CURL="${tmp}/curl" CALLS="${tmp}/calls.log" STATE="${tmp}/state" \
    STATE_DIR="${tmp}/state" FAKE_DIGEST=new FAKE_PREVIOUS="$2" FAKE_HEALTHY="$3" \
    HEALTH_TRIES=2 HEALTH_SLEEP=0 "${here}/deploy-server.sh" "$1" >"${tmp}/out.log" 2>&1
}
expect() { # name want_rc got_rc want_current_regex(空=不检查)
  local cur
  cur="$(cat "${tmp}/state/current" 2>/dev/null)"
  if [ "$2" != "$3" ]; then echo "FAIL $1: 退出码 期望 $2 实际 $3"; cat "${tmp}/out.log"; fail=1; return; fi
  if [ -n "$4" ] && ! grep -Eq "$4" <<<"${cur}"; then echo "FAIL $1: 运行中镜像 期望匹配 $4 实际 ${cur}"; fail=1; return; fi
  echo "ok   $1"
}

ref=ghcr.io/vod-studio/kite-server:v1.2.3
old=ghcr.io/vod-studio/kite-server@sha256:old

deploy "${ref}" "${old}" "sha256:new" ""
expect "新版本健康 → 部署成功,按 digest 运行" 0 $? "@sha256:new$"
grep -q " v1.2.3 ghcr.io/vod-studio/kite-server@sha256:new$" "${tmp}/state/deploy-history" ||
  { echo "FAIL 成功后应记录部署历史"; fail=1; }
grep -q "^login" "${tmp}/calls.log" && { echo "FAIL 没有令牌时不应登录"; fail=1; }
grep -qx "network create --ignore kite_network" "${tmp}/calls.log" ||
  { echo "FAIL 应确保专用网络 kite_network 存在"; fail=1; }
grep -q -- "--network kite_network --network-alias kite" <(grep '^run ' "${tmp}/calls.log") ||
  { echo "FAIL 容器应加入 kite_network,别名 kite"; fail=1; }
[ "$(grep -c -- '--network ' "${tmp}/calls.log")" = 1 ] ||
  { echo "FAIL 容器只应加入一个网络"; fail=1; }

deploy "${ref}" "${old}" "sha256:old" ""
expect "新版本不健康 → 回滚到旧镜像" 1 $? "@sha256:old$"
[ ! -e "${tmp}/state/deploy-history" ] || { echo "FAIL 失败的部署不应记录历史"; fail=1; }

FAKE_RUN_FAIL="sha256:new" deploy "${ref}" "${old}" "sha256:old" ""
expect "新容器启动失败(旧容器已删除)→ 仍回滚到旧镜像" 1 $? "@sha256:old$"

deploy "${ref}" "" "never" ""
expect "首次部署失败 → 退出 1,不回滚" 1 $? ""
[ "$(grep -c '^run ' "${tmp}/calls.log")" = 1 ] || { echo "FAIL 首次部署失败不应再启动别的镜像"; fail=1; }

deploy "ghcr.io/vod-studio/kite-server:latest" "${old}" "sha256:new" ""
expect "非版本 tag → 拒绝" 2 $? ""
[ ! -s "${tmp}/calls.log" ] || { echo "FAIL 拒绝时不应调用 podman"; fail=1; }

deploy "${ref}; rm -rf /" "${old}" "sha256:new" ""
expect "带额外内容的引用 → 拒绝" 2 $? ""

deploy "${ref}" "${old}" "sha256:new" "ghs_token123"
expect "带令牌 → 先登录再拉取" 0 $? "@sha256:new$"
[ "$(cat "${tmp}/state/login-token")" = "ghs_token123" ] || { echo "FAIL 令牌应经 stdin 交给 podman login"; fail=1; }
grep -q -- "--authfile" <(grep '^pull' "${tmp}/calls.log") || { echo "FAIL 拉取应使用临时 authfile"; fail=1; }

# 闸门:只放行合法引用,并原样交给 sudo 执行部署脚本
gate() { # ssh_original_command
  : >"${tmp}/calls.log"
  SSH_ORIGINAL_COMMAND="$1" SUDO="${tmp}/sudo" CALLS="${tmp}/calls.log" DEPLOY_SERVER=/x/deploy-server.sh \
    "${here}/deploy-ssh-gate.sh" </dev/null >"${tmp}/out.log" 2>&1
}
gate "${ref}"
rc=$?
if [ "${rc}" = 0 ] && grep -qx "sudo -n /x/deploy-server.sh ${ref}" "${tmp}/calls.log"; then
  echo "ok   闸门:合法引用交给 sudo 执行"
else
  echo "FAIL 闸门:合法引用应交给 sudo(rc=${rc})"; fail=1
fi
for bad in "" "bash" "${ref} && id" "ghcr.io/other/image:v1.0.0"; do
  gate "${bad}"
  rc=$?
  if [ "${rc}" = 2 ] && [ ! -s "${tmp}/calls.log" ]; then
    echo "ok   闸门:拒绝 '${bad}'"
  else
    echo "FAIL 闸门:应拒绝 '${bad}'(rc=${rc})"; fail=1
  fi
done

exit "${fail}"
