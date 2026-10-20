#!/usr/bin/env bash
# 与 HEAD 比较 staged/unstaged 以及未跟踪文件，不遗漏新生成的 API。
set -euo pipefail
if [ "$#" -eq 0 ]; then set -- gen/; fi
changes=$(git status --porcelain --untracked-files=all -- "$@")
if [ -n "$changes" ]; then
  printf '%s\n' "$changes" >&2
  echo '生成产物有漂移，请重新生成并提交全部产物。' >&2
  exit 1
fi
