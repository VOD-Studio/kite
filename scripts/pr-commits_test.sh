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
git init -q "$tmp/repo"
git -C "$tmp/repo" config user.name 'PR test'
git -C "$tmp/repo" config user.email 'pr-test@example.invalid'
printf '%s\n' '{".":"0.4.0","packages/kite-player":"0.1.0"}' >"$tmp/repo/.release-please-manifest.json"
git -C "$tmp/repo" add .
git -C "$tmp/repo" commit -qm 'chore: initial versions'
PR_BASE_SHA=$(git -C "$tmp/repo" rev-parse HEAD)
export PR_BASE_SHA PR_HEAD_SHA="$PR_BASE_SHA"
printf '%s\n' '{".":"0.5.0","packages/kite-player":"0.1.0"}' >"$tmp/repo/.release-please-manifest.json"
git -C "$tmp/repo" commit -qam 'chore(kite): release 0.5.0'
kite_head=$(git -C "$tmp/repo" rev-parse HEAD)
git -C "$tmp/repo" checkout -q --detach "$PR_BASE_SHA"
printf '%s\n' '{".":"0.4.0","packages/kite-player":"0.2.0"}' >"$tmp/repo/.release-please-manifest.json"
git -C "$tmp/repo" commit -qam 'chore(player): release 0.2.0'
player_head=$(git -C "$tmp/repo" rev-parse HEAD)
normal_pr() {
  PR_HEAD_SHA=$PR_BASE_SHA
  printf '%s\n' '{"base":{"ref":"main"},"head":{"ref":"feat/topic","repo":{"full_name":"contributor/kite"}},"user":{"login":"contributor","type":"User"},"title":"feat(cli): 新功能","commits":2}' >"$CASE_PR"
}
expect() {
  local name=$1 want=$2 rc=0
  jq --arg head "$PR_HEAD_SHA" '.head.sha=$head' "$CASE_PR" >"$tmp/changed"
  cp "$tmp/changed" "$CASE_PR"
  (cd "$tmp/repo" && "$here/pr-commits.sh") >"$tmp/log" 2>&1 || rc=$?
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
PR_HEAD_SHA=$kite_head
printf '%s\n' '{"base":{"ref":"main","repo":{"full_name":"VOD-Studio/kite"}},"head":{"ref":"release-please--branches--main--components--kite","repo":{"full_name":"attacker/kite"}},"user":{"login":"attacker","type":"User"},"title":"chore(kite): release 0.5.0","commits":1}' >"$CASE_PR"
printf '%s\n' '[{"sha":"a","commit":{"message":"chore(kite): release 0.5.0"},"parents":[{}]}]' >"$CASE_COMMITS"
expect '同名 fork 分支不能冒充 release PR' 1
printf '%s\n' '{"base":{"ref":"main","repo":{"full_name":"VOD-Studio/kite"}},"head":{"ref":"release-please--branches--main--components--kite","repo":{"full_name":"VOD-Studio/kite"}},"user":{"login":"github-actions[bot]","type":"Bot"},"title":"chore(kite): release 0.5.0","commits":1}' >"$CASE_PR"
expect '本仓库机器人 release PR 可进入人工 squash 合并' 0
jq '.title="chore(main): release 0.5.0"' "$CASE_PR" >"$tmp/changed"; cp "$tmp/changed" "$CASE_PR"
expect '新 Go release PR 不接受历史标题' 1
PR_HEAD_SHA=$player_head
printf '%s\n' '{"base":{"ref":"main","repo":{"full_name":"VOD-Studio/kite"}},"head":{"ref":"release-please--branches--main--components--player","repo":{"full_name":"VOD-Studio/kite"}},"user":{"login":"github-actions[bot]","type":"Bot"},"title":"chore(player): release 0.2.0","commits":1}' >"$CASE_PR"
expect 'player release PR 使用独立组件身份' 0
jq '.title="chore(main): release player 0.2.0"' "$CASE_PR" >"$tmp/changed"; cp "$tmp/changed" "$CASE_PR"
expect '新 player release PR 不接受历史标题' 1
jq '.title="chore(kite): release 0.2.0"' "$CASE_PR" >"$tmp/changed"; cp "$tmp/changed" "$CASE_PR"
expect 'player分支不能冒充Go release标题' 1
normal_pr
jq '.title="wip" | .commits=1' "$CASE_PR" >"$tmp/changed"; cp "$tmp/changed" "$CASE_PR"
expect '合法提交不能掩盖无效PR标题' 1
python3 "$here/pr-version-state_test.py"
