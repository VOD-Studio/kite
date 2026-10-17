#!/usr/bin/env bash
# govulncheck 门禁判定(原内联在 ci.yml 的 security job,抽出后本地与 CI 共用)。
#
# 有修复版本的漏洞 → 失败;Fixed-in-N/A 或修复版本尚未进入 toolchain 清单 → 降级 warning。
# 非零退出且输出无漏洞报告特征 = 工具自身执行错误(编译失败/网络等),必须失败:
# 吞成绿色 warning 会让扫描形同虚设(violet #138 教训)。
#
# 用法: scripts/govulncheck-gate.sh [govulncheck 包模式,默认 ./...]
set -uo pipefail

PKGS="${1:-./...}"
GOVULNCHECK="${GOVULNCHECK:-$(command -v govulncheck || echo "$(go env GOPATH)/bin/govulncheck")}"

if [ ! -x "${GOVULNCHECK}" ]; then
  echo "::error::未找到 govulncheck,跑 'make install-tools' 安装"
  exit 1
fi

output=$("${GOVULNCHECK}" "${PKGS}" 2>&1)
code=$?
echo "${output}"

[ "${code}" -eq 0 ] && exit 0

# here-string 而非管道:grep -q 提前退出会让 echo 收到 SIGPIPE,pipefail 下被误判为失败。
if ! grep -qE "Vulnerability #[0-9]|Fixed in:" <<<"${output}"; then
  echo "::error::govulncheck 执行失败(exit ${code}),输出无漏洞报告特征,疑为工具自身错误"
  exit 1
fi

# 提取有修复版本的漏洞(Fixed in: 非 N/A)。
fixable=$(echo "${output}" | grep "Fixed in:" | grep -v "N/A" || true)

if [ -n "${fixable}" ]; then
  # fix 版本可能尚未进入 setup-go 清单(Go 刚发布/即将发布的窗口期):
  # govulncheck DB 从 release dashboard prepared 态就提前标记 fix 版本,
  # 但 runner 装不到,升级无门。fix 版本严格高于当前 toolchain 时降级 warning;
  # setup-go 清单同步后 toolchain 升级,漏洞自动消失。
  current_go=$(go version | grep -oE 'go[0-9]+\.[0-9]+\.[0-9]+' || true)
  if [ -z "${current_go}" ]; then
    # 解析失败(rc/devel 等非三段版本号)时跳过降级、保持 error:
    # 窗口期降级的 fail-safe 方向是宁可误报,不可漏报可修复漏洞。
    echo "::warning::无法解析当前 Go 版本,跳过窗口期降级(保持 error 语义)"
  else
    for fix_ver in $(echo "${fixable}" | grep -oE 'go[0-9]+\.[0-9]+\.[0-9]+' | sort -u); do
      if [ "$(printf '%s\n%s\n' "${current_go}" "${fix_ver}" | sort -V | tail -1)" = "${fix_ver}" ] \
        && [ "${current_go}" != "${fix_ver}" ]; then
        echo "::warning::${fix_ver} 尚未进入 setup-go 清单(当前 ${current_go}),相关漏洞降级为 advisory"
        # $ 锚定行尾:govulncheck 格式为 "Fixed in: module@version",
        # 避免子串误删(go1.26.6 误删 go1.26.60 的行)。
        fixable=$(echo "${fixable}" | grep -v "@${fix_ver}\$" || true)
      fi
    done
  fi
fi

if [ -n "${fixable}" ]; then
  echo "::error::发现可修复的漏洞,请升级依赖:"
  echo "${fixable}"
  exit 1
fi
echo "::warning::所有漏洞均为 Fixed-in-N/A 或 fix 版本尚未进入 toolchain 清单,降级为 advisory"
