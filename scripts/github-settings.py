#!/usr/bin/env python3
"""管理员显式配置 GitHub 边界；默认只检查，不读取或迁移任何 secret 值。"""
import argparse
import json
import os
import subprocess
import sys

REPO = "VOD-Studio/kite"
ROOT = f"repos/{REPO}"
DEPLOY_SECRETS = {"KITE_PRODUCTION_SSH_KEY", "KITE_PRODUCTION_KNOWN_HOSTS", "KITE_PRODUCTION_HOST"}
LEGACY_DEPLOY_SECRETS = {"KITE_DEPLOY_SSH_KEY", "KITE_DEPLOY_KNOWN_HOSTS", "KITE_DEPLOY_HOST", "KITE_DEPLOY_PORT"}


def api(path, method="GET", body=None, missing_ok=False):
    command = [os.environ.get("GH", "gh"), "api", "--method", method, path]
    if body is not None:
        command += ["--input", "-"]
    result = subprocess.run(command, input=None if body is None else json.dumps(body), text=True, capture_output=True, check=False)
    if result.returncode:
        if missing_ok and "HTTP 404" in result.stderr:
            return None
        raise RuntimeError(f"GitHub {method} {path} 失败: {result.stderr.strip()}")
    return json.loads(result.stdout) if result.stdout.strip() else None


def find_ruleset(name, listed):
    matches = [item for item in listed if item["name"] == name and item.get("source_type", "Repository") == "Repository"]
    if len(matches) > 1:
        raise RuntimeError(f"有多个 {name} ruleset,请先由管理员消除歧义")
    return api(f"{ROOT}/rulesets/{matches[0]['id']}") if matches else None


def desired_main(existing):
    rules = [] if existing is None else existing["rules"]
    checks = []
    for rule in rules:
        if rule["type"] == "required_status_checks":
            checks.extend(rule["parameters"]["required_status_checks"])
    checks = [check for check in checks if check["context"] != "CI OK"]
    checks.append({"context": "CI OK", "integration_id": 15368})
    # 保留未知/人工追加的规则；线性历史会禁止用户选择普通 merge,因此显式移除。
    replaced = {"pull_request", "required_status_checks", "required_linear_history"}
    result = [rule for rule in rules if rule["type"] not in replaced]
    for kind in ("deletion", "non_fast_forward"):
        if not any(rule["type"] == kind for rule in result):
            result.append({"type": kind})
    result += [
        {"type": "pull_request", "parameters": {
            "dismiss_stale_reviews_on_push": True, "require_code_owner_review": False,
            "require_last_push_approval": False, "required_approving_review_count": 0,
            "required_review_thread_resolution": True,
        }},
        {"type": "required_status_checks", "parameters": {
            "do_not_enforce_on_create": False, "required_status_checks": checks,
            "strict_required_status_checks_policy": False,
        }},
    ]
    return {"name": "Protect main", "target": "branch", "enforcement": "active", "bypass_actors": [],
            "conditions": {"ref_name": {"include": ["~DEFAULT_BRANCH"], "exclude": []}}, "rules": result}


def desired_tags(existing):
    rules = [] if existing is None else list(existing["rules"])
    for kind in ("deletion", "update"):
        if not any(rule["type"] == kind for rule in rules):
            rules.append({"type": kind})
    return {"name": "Protect release tags", "target": "tag", "enforcement": "active", "bypass_actors": [],
            "conditions": {"ref_name": {"include": ["refs/tags/v*"], "exclude": []}}, "rules": rules}


def put_ruleset(existing, desired):
    path = f"{ROOT}/rulesets"
    if existing is not None:
        api(f"{path}/{existing['id']}", "PUT", desired)
    else:
        api(path, "POST", desired)


def inspect():
    repo = api(ROOT)
    listed = api(f"{ROOT}/rulesets?includes_parents=false")
    main = find_ruleset("Protect main", listed)
    tags = find_ruleset("Protect release tags", listed)
    env = api(f"{ROOT}/environments/production", missing_ok=True)
    policy = api(f"{ROOT}/actions/permissions")
    workflow = api(f"{ROOT}/actions/permissions/workflow")
    return repo, main, tags, env, policy, workflow


def configure(state):
    _, main, tags, environment, policy, workflow = state
    if environment is not None and any(
        rule["type"] in ("required_reviewers", "wait_timer") for rule in environment.get("protection_rules", [])
    ):
        raise RuntimeError("production 仍有人工审批/等待规则;请管理员先审核处理,未应用任何配置")
    api(ROOT, "PATCH", {"allow_merge_commit": True, "allow_squash_merge": True, "allow_rebase_merge": True,
                       "allow_auto_merge": False, "squash_merge_commit_title": "PR_TITLE", "squash_merge_commit_message": "COMMIT_MESSAGES"})
    put_ruleset(main, desired_main(main))
    put_ruleset(tags, desired_tags(tags))
    api(f"{ROOT}/actions/permissions", "PUT", {"enabled": True, "allowed_actions": policy["allowed_actions"], "sha_pinning_required": True})
    api(f"{ROOT}/actions/permissions/workflow", "PUT", {
        "default_workflow_permissions": "read",
        # GitHub 此开关也影响机器人创建 release PR,保留现有设置。
        "can_approve_pull_request_reviews": workflow["can_approve_pull_request_reviews"],
    })
    api(f"{ROOT}/environments/production", "PUT", {"deployment_branch_policy": {
        "protected_branches": False, "custom_branch_policies": True,
    }})
    policies = api(f"{ROOT}/environments/production/deployment-branch-policies")["branch_policies"]
    found_main = False
    for item in policies:
        if item["name"] == "main" and item.get("type", "branch") == "branch":
            found_main = True
        else:
            api(f"{ROOT}/environments/production/deployment-branch-policies/{item['id']}", "DELETE")
    if not found_main:
        api(f"{ROOT}/environments/production/deployment-branch-policies", "POST", {"name": "main", "type": "branch"})


def rules_match(actual, expected):
    # GitHub 可补默认字段或调整 rule 顺序；只比较我们管理的语义。
    for wanted in expected:
        candidates = [rule for rule in actual if rule["type"] == wanted["type"]]
        if not candidates:
            return False
        params = candidates[0].get("parameters", {})
        for key, value in wanted.get("parameters", {}).items():
            found = params.get(key)
            if key == "required_status_checks":
                if not isinstance(found, list) or not all(
                    any(all(check.get(k) == v for k, v in required.items()) for check in found)
                    for required in value
                ):
                    return False
            elif found != value:
                return False
    return not any(rule["type"] == "required_linear_history" for rule in actual)


def audit(state):
    repo, main, tags, env, policy, workflow = state
    problems = []
    if not all(repo.get(key) for key in ("allow_merge_commit", "allow_squash_merge", "allow_rebase_merge")) or repo.get("allow_auto_merge"):
        problems.append("合并设置应保留三种方式并关闭自动合并")
    if main is None:
        problems.append("缺少 Protect main ruleset")
    else:
        expected = desired_main(main)
        for key in ("target", "enforcement", "bypass_actors", "conditions"):
            if main.get(key) != expected[key]:
                problems.append(f"Protect main 的 {key} 不符合受保护主干契约")
        if not rules_match(main["rules"], expected["rules"]):
            problems.append("Protect main 缺少必要 PR/CI 规则或禁止普通 merge")
    if tags is None:
        problems.append("缺少 Protect release tags ruleset")
    else:
        expected = desired_tags(tags)
        for key in ("target", "enforcement", "bypass_actors", "conditions"):
            if tags.get(key) != expected[key]:
                problems.append(f"Protect release tags 的 {key} 不符合不可变版本契约")
        if not rules_match(tags["rules"], expected["rules"]):
            problems.append("版本 tag 缺少更新/删除保护")
    if not policy.get("sha_pinning_required") or not policy.get("enabled"):
        problems.append("Actions 必须启用并要求第三方 Action 固定完整 SHA")
    if workflow.get("default_workflow_permissions") != "read":
        problems.append("GITHUB_TOKEN 默认权限必须为 read")
    if env is None:
        problems.append("缺少 production 环境")
    else:
        if env.get("deployment_branch_policy") != {"protected_branches": False, "custom_branch_policies": True}:
            problems.append("production 必须使用明确分支白名单")
        else:
            policies = api(f"{ROOT}/environments/production/deployment-branch-policies")["branch_policies"]
            if [(item["name"], item.get("type", "branch")) for item in policies] != [("main", "branch")]:
                problems.append("production 只允许 main 分支,不允许 tag 或任意分支")
        if any(rule["type"] in ("required_reviewers", "wait_timer") for rule in env.get("protection_rules", [])):
            problems.append("production 存在额外人工审批/等待规则,与 release PR 合并后自动上线的契约不符;请管理员审核后调整")
        names = {item["name"] for item in api(f"{ROOT}/environments/production/secrets?per_page=100")["secrets"]}
        for name in sorted(DEPLOY_SECRETS - names):
            problems.append(f"production 环境缺少 {name};管理员需重新设置,脚本不能读取或转移原 secret 值")
    forbidden = DEPLOY_SECRETS | {"KITE_PRODUCTION_PORT"} | LEGACY_DEPLOY_SECRETS
    for scope, endpoint in (("仓库", "secrets"), ("组织", "organization-secrets")):
        names = {item["name"] for item in api(f"{ROOT}/actions/{endpoint}?per_page=100")["secrets"]}
        for name in sorted(names & forbidden):
            problems.append(f"{scope}级存在 {name};部署凭据只能配置到 production,迁移验证后由管理员移除旧项,避免同名 secret 回退")
    for problem in problems:
        print(f"未满足: {problem}")
    if not problems:
        print("ok: 合并选择、主干门禁、版本 tag、Action 固定及 production 凭据边界")
    return 1 if problems else 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true", help="显式修改本仓库保护配置;不迁移/删除任何 secret")
    args = parser.parse_args()
    try:
        state = inspect()
        if args.apply:
            configure(state)
            state = inspect()
        return audit(state)
    except (RuntimeError, ValueError, KeyError) as error:
        print(f"配置检查失败: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
