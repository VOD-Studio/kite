#!/usr/bin/env bash
# 在持久化的隔离 GitHub API 边界验证管理员配置:默认只读、显式应用、保留未知规则、不碰 secret。
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export SETTINGS_STATE="$tmp/state.json" SETTINGS_WRITES="$tmp/writes" GH="$tmp/gh"
cat >"$GH" <<'PY'
#!/usr/bin/env python3
import json, os, sys
args = sys.argv[1:]
method = args[args.index('--method') + 1]
path = args[args.index('--method') + 2].removeprefix('repos/VOD-Studio/kite').split('?')[0]
body = json.load(sys.stdin) if '--input' in args else None
with open(os.environ['SETTINGS_STATE']) as f:
    state = json.load(f)
if method != 'GET':
    with open(os.environ['SETTINGS_WRITES'], 'a') as f:
        f.write(method + ' ' + path + '\n')
    if 'secrets' in path:
        sys.exit('secret 不允许写入或删除')
result = None
if path == '':
    if body is not None: state['repo'].update(body)
    result = state['repo']
elif path == '/rulesets':
    if method == 'POST':
        key = str(max([int(k) for k in state['rulesets']] + [0]) + 1)
        state['rulesets'][key] = dict(body, id=int(key))
    result = list(state['rulesets'].values())
elif path.startswith('/rulesets/'):
    key = path.rsplit('/', 1)[1]
    if body is not None: state['rulesets'][key] = dict(body, id=int(key))
    result = state['rulesets'][key]
elif path in ('/actions/permissions', '/actions/permissions/workflow'):
    key = 'policy' if path.endswith('/permissions') else 'workflow'
    if body is not None: state[key].update(body)
    result = state[key]
elif path == '/environments/production':
    if body is not None:
        if state['env'] is None: state['env'] = {'protection_rules': []}
        state['env'].update(body)
    if state['env'] is None:
        print('Not Found (HTTP 404)', file=sys.stderr)
        sys.exit(1)
    result = state['env']
elif path == '/environments/production/deployment-branch-policies':
    if method == 'POST':
        state['branches'].append(dict(body, id=99))
    result = {'branch_policies': state['branches']}
elif path.startswith('/environments/production/deployment-branch-policies/'):
    key = int(path.rsplit('/', 1)[1])
    state['branches'] = [item for item in state['branches'] if item['id'] != key]
elif path == '/environments/production/secrets':
    result = {'secrets': [{'name': name} for name in state['env_secrets']]}
elif path == '/actions/secrets':
    result = {'secrets': [{'name': name} for name in state['repo_secrets']]}
elif path == '/actions/organization-secrets':
    result = {'secrets': [{'name': name} for name in state.get('org_secrets', [])]}
else:
    sys.exit('unexpected endpoint: ' + path)
with open(os.environ['SETTINGS_STATE'], 'w') as f:
    json.dump(state, f)
if result is not None: print(json.dumps(result))
PY
chmod +x "$GH"
cat >"$SETTINGS_STATE" <<'JSON'
{"repo":{"allow_merge_commit":false,"allow_squash_merge":true,"allow_rebase_merge":false,"allow_auto_merge":true},"rulesets":{"1":{"id":1,"name":"Protect main","target":"branch","enforcement":"active","bypass_actors":[],"conditions":{"ref_name":{"include":["~DEFAULT_BRANCH"],"exclude":[]}},"rules":[{"type":"required_signatures"},{"type":"required_linear_history"}]}},"env":null,"branches":[],"policy":{"enabled":true,"allowed_actions":"selected","sha_pinning_required":false},"workflow":{"default_workflow_permissions":"write","can_approve_pull_request_reviews":true},"env_secrets":[],"repo_secrets":["KITE_DEPLOY_SSH_KEY"]}
JSON
if python3 "$here/github-settings.py" >"$tmp/log" 2>&1; then echo 'FAIL 不安全配置不能被报告为就绪'; exit 1; fi
[ ! -e "$SETTINGS_WRITES" ] || { echo 'FAIL 默认检查不应修改 GitHub'; exit 1; }
echo 'ok   默认只读并识别不完整保护'
if python3 "$here/github-settings.py" --apply >"$tmp/log" 2>&1; then echo 'FAIL 缺生产环境凭据不能假成功'; exit 1; fi
python3 - <<'PY'
import json, os
with open(os.environ['SETTINGS_STATE']) as f: s = json.load(f)
assert all(s['repo'][k] for k in ('allow_merge_commit','allow_squash_merge','allow_rebase_merge'))
assert not s['repo']['allow_auto_merge']
rules = {r['type']:r for r in s['rulesets']['1']['rules']}
assert 'required_signatures' in rules, '人工增加的规则必须保留'
assert 'required_linear_history' not in rules, '不可禁止普通 merge'
assert 'pull_request' in rules and 'required_status_checks' in rules
assert s['policy']['allowed_actions'] == 'selected', '不得放宽上游 Action 白名单'
assert s['policy']['sha_pinning_required']
assert s['workflow']['can_approve_pull_request_reviews'], '不得意外禁用 release bot 创建 PR'
assert s['repo_secrets'] == ['KITE_DEPLOY_SSH_KEY'], '不得提前删除仍被生产使用的密钥'
assert [(p['name'],p['type']) for p in s['branches']] == [('main','branch')]
# 模拟管理员完成独立凭据迁移,不是脚本读取/复制 secret 值。
s['env_secrets'] = ['KITE_PRODUCTION_SSH_KEY','KITE_PRODUCTION_KNOWN_HOSTS','KITE_PRODUCTION_HOST']
s['repo_secrets'] = []
with open(os.environ['SETTINGS_STATE'], 'w') as f: json.dump(s,f)
PY
python3 "$here/github-settings.py" >"$tmp/log" 2>&1 || { cat "$tmp/log"; exit 1; }
cp "$SETTINGS_STATE" "$tmp/complete.json"
python3 "$here/github-settings.py" --apply >"$tmp/log" 2>&1 || { cat "$tmp/log"; exit 1; }
python3 - "$tmp/complete.json" <<'PY'
import json, os, sys
with open(sys.argv[1]) as f: before = json.load(f)
with open(os.environ['SETTINGS_STATE']) as f: after = json.load(f)
assert before == after, '重复应用应保持相同配置,不能重复增加规则或分支'
PY
echo 'ok   显式配置保留人工规则/合并选择,凭据独立迁移后幂等通过'
for scope in repo_secrets org_secrets; do
  python3 - "$scope" <<'PY'
import json, os, sys
with open(os.environ['SETTINGS_STATE']) as f: s = json.load(f)
s['repo_secrets'] = []
s['org_secrets'] = []
s[sys.argv[1]] = ['KITE_PRODUCTION_SSH_KEY']
with open(os.environ['SETTINGS_STATE'], 'w') as f: json.dump(s,f)
PY
  if python3 "$here/github-settings.py" >"$tmp/log" 2>&1; then
    echo "FAIL ${scope} 的同名部署凭据不能被环境配置掩盖"; exit 1
  fi
done
echo 'ok   仓库/组织的同名 production 密钥会被拒绝'
python3 - <<'PY'
import json, os
with open(os.environ['SETTINGS_STATE']) as f: s = json.load(f)
s['org_secrets'] = []
s['env']['protection_rules'] = [{'type': 'wait_timer', 'wait_timer': 12}]
with open(os.environ['SETTINGS_STATE'], 'w') as f: json.dump(s,f)
PY
cp "$SETTINGS_WRITES" "$tmp/before-protected-apply"
if python3 "$here/github-settings.py" --apply >"$tmp/log" 2>&1; then
  echo 'FAIL 现有人工作业门禁必须由管理员先处理'; exit 1
fi
cmp -s "$SETTINGS_WRITES" "$tmp/before-protected-apply" || {
  echo 'FAIL 人工门禁冲突时不得先修改其它设置或重写环境'; exit 1
}
echo 'ok   现有人工部署门禁在任何配置修改之前阻止 apply'
