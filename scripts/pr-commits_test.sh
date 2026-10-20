#!/usr/bin/env bash
# PR 提交策略:保留原子提交,不把三种合并选择限制成一律 squash。
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cat >"$tmp/gh" <<'SH'
#!/usr/bin/env bash
case "$*" in
  *'/pulls/42/commits?'*) cat "$CASE_COMMITS" ;;
  *'/pulls/42') cat "$CASE_PR" ;;
  *) exit 90 ;;
esac
SH
chmod +x "$tmp/gh"
export GH="$tmp/gh" GH_REPO=VOD-Studio/kite PR_NUMBER=42 CASE_PR="$tmp/pr.json" CASE_COMMITS="$tmp/commits.json"
normal_pr() {
  printf '%s\n' '{"base":{"ref":"main"},"head":{"ref":"feat/topic","repo":{"full_name":"contributor/kite"}},"user":{"login":"contributor","type":"User"},"title":"feat(cli): 新功能","commits":2}' >"$CASE_PR"
}
expect() {
  local name=$1 want=$2 rc=0
  "$here/pr-commits.sh" >"$tmp/log" 2>&1 || rc=$?
  if [ "$rc" != "$want" ]; then printf 'FAIL %s: exit %s, want %s\n' "$name" "$rc" "$want"; cat "$tmp/log"; exit 1; fi
  printf 'ok   %s\n' "$name"
}
normal_pr
printf '%s\n' '[{"sha":"a","commit":{"message":"feat(cli): 新功能"},"parents":[{}]},{"sha":"b","commit":{"message":"fix(cli): 修正边界"},"parents":[{}]}]' >"$CASE_COMMITS"
expect '普通 PR 可保留多条原子提交' 0
printf '%s\n' '[{"sha":"a","commit":{"message":"feat(cli): 新功能"},"parents":[{}]},{"sha":"b","commit":{"message":"Merge branch main"},"parents":[{},{}]}]' >"$CASE_COMMITS"
expect '真正的合并节点不要求伪装成业务提交' 0
printf '%s\n' '[{"sha":"a","commit":{"message":"feat(cli): 新功能"},"parents":[{}]},{"sha":"b","commit":{"message":"wip"},"parents":[{}]}]' >"$CASE_COMMITS"
expect '非规范原子提交不得靠 PR 标题掩盖' 1
printf '%s\n' '[{"sha":"a","commit":{"message":"feat(cli): 新功能"},"parents":[{}]}]' >"$CASE_COMMITS"
expect 'API 截断提交列表不得误报完成验证' 1
printf '%s\n' '{"base":{"ref":"main"},"head":{"ref":"release-please--branches--main","repo":{"full_name":"attacker/kite"}},"user":{"login":"attacker","type":"User"},"title":"chore(main): release 0.5.0","commits":1}' >"$CASE_PR"
printf '%s\n' '[{"sha":"a","commit":{"message":"chore(main): release 0.5.0"},"parents":[{}]}]' >"$CASE_COMMITS"
expect '同名 fork 分支不能冒充 release PR' 1
printf '%s\n' '{"base":{"ref":"main"},"head":{"ref":"release-please--branches--main","repo":{"full_name":"VOD-Studio/kite"}},"user":{"login":"github-actions[bot]","type":"Bot"},"title":"chore(main): release 0.5.0","commits":1}' >"$CASE_PR"
expect '本仓库机器人 release PR 可进入人工 squash 合并' 0
