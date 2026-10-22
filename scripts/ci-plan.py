#!/usr/bin/env python3
"""Plan real changed files, then independently rederive that plan at the CI gate."""
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys

import yaml

# This is the only task registry. Workflow conditions and the gate consume it.
TASKS = {
    "control": "control", "go-quality": "go", "go-test": "go",
    "windows-vet": "go", "player": "player", "proto": "contract",
    "go-interop": "contract", "rust-interop": "contract",
    "go-package": "package", "security": "security", "metadata": "metadata",
}
MODULES = ("go", "player", "rust")
SHA = re.compile(r"[0-9a-f]{40}\Z")


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def sha(value):
    if not isinstance(value, str) or not SHA.fullmatch(value):
        raise ValueError("missing or invalid source SHA")
    return value


def dependency_modules(paths):
    modules = set()
    for path in paths:
        if path in ("go.mod", "go.sum", "interop/go/go.mod", "interop/go/go.sum"):
            modules.add("go")
        if path in ("packages/kite-player/package.json", "packages/kite-player/package-lock.json"):
            modules.add("player")
        if path in ("interop/rust/Cargo.toml", "interop/rust/Cargo.lock", "interop/rust/rust-toolchain.toml"):
            modules.add("rust")
        if control_path(path):
            modules.update(MODULES)
    return [module for module in MODULES if module in modules]


def control_path(path):
    return path == "Makefile" or path.startswith(("scripts/", ".github/"))


def selected_tasks(paths, full=False):
    selected = set(TASKS) - {"security", "metadata"} if full else set()
    for path in paths:
        if control_path(path):
            selected.update(set(TASKS) - {"metadata", "security"})
        elif path.startswith("packages/kite-player/"):
            selected.add("player")
            # The REST DTO/source adapter is shared with internal/public.
            if path in ("packages/kite-player/src/core/types.ts", "packages/kite-player/src/core/source.ts"):
                selected.update(("go-quality", "go-test", "windows-vet"))
        elif path.startswith(("proto/", "gen/")):
            selected.update(set(TASKS) - {"metadata", "security", "control"})
        elif path.startswith("internal/public/"):
            selected.update(("go-quality", "go-test", "windows-vet", "go-package", "player"))
        elif path.startswith("interop/go/"):
            selected.update(("go-quality", "go-test", "go-interop"))
        elif path.startswith("interop/rust/"):
            selected.update(("go-test", "rust-interop"))
        elif path == "docs/player.md":
            selected.update(("go-test", "player"))
        elif path == "docs/integration.md":
            selected.update(("go-test", "go-interop", "rust-interop"))
        elif path in ("docs/grpc-errors.md", "docs/server-security.md"):
            selected.update(("go-test", "rust-interop"))
        elif path.startswith("docs/cmd/") or path == "llms.txt":
            selected.add("go-test")
        elif (path.startswith("docs/") and path.endswith(".md")) or path in (
            "README.md", "CONTRIBUTING.md", "CHANGELOG.md", "SECURITY.md",
            "AGENTS.md", "CONTEXT.md", "LICENSE",
        ):
            pass
        elif path.startswith(("cmd/", "internal/", "config/", "observability/", "errors/")) or path in (
            "go.mod", "go.sum", ".golangci.yaml", "config.example.yaml", ".air.toml",
        ):
            selected.update(("go-quality", "go-test", "windows-vet", "go-package", "go-interop", "rust-interop"))
        elif path in ("Dockerfile", "Dockerfile.release", ".dockerignore", ".goreleaser.yaml"):
            selected.add("go-package")
        else:
            # New/unclassified paths cannot quietly opt out of a check.
            selected.update(set(TASKS) - {"metadata", "security"})
    return selected


def validate_workflow():
    # needs only contains explicitly listed jobs. Also inspect the full workflow,
    # otherwise a newly added job omitted from needs could fail behind a green gate.
    workflow = yaml.safe_load(Path(".github/workflows/ci.yml").read_text())
    jobs = workflow.get("jobs") if isinstance(workflow, dict) else None
    if not isinstance(jobs, dict) or set(jobs) != {"plan", "ci-ok", *TASKS}:
        raise ValueError("workflow contains a missing or unregistered CI job")
    needs = jobs["ci-ok"].get("needs")
    if not isinstance(needs, list) or len(needs) != len(set(needs)) or set(needs) != {"plan", *TASKS}:
        raise ValueError("CI OK must depend on the plan and every registered job")


def changed_files(base, head, merge_base=False):
    sha(base)
    sha(head)
    revision = f"{base}...{head}" if merge_base else f"{base}..{head}"
    # --no-renames includes BOTH the deleted and added path in a cross-scope move.
    raw = subprocess.check_output(["git", "diff", "--name-only", "--no-renames", "-z", revision])
    paths = raw.decode("utf-8").split("\0")
    result = sorted(set(path for path in paths if path))
    for path in result:
        parsed = PurePosixPath(path)
        if parsed.is_absolute() or ".." in parsed.parts:
            raise ValueError("unsafe changed path")
    return result


def github_api(route):
    repo = os.environ["GITHUB_REPOSITORY"]
    return json.loads(command("gh", "api", f"repos/{repo}/{route}"))


def context():
    event_name = os.environ["GITHUB_EVENT_NAME"]
    source = sha(os.environ["GITHUB_SHA"])
    ref = os.environ["GITHUB_REF"]
    run_id = os.environ["GITHUB_RUN_ID"]
    attempt = os.environ["GITHUB_RUN_ATTEMPT"]
    if not run_id.isdecimal() or int(run_id) <= 0 or not attempt.isdecimal() or int(attempt) <= 0:
        raise ValueError("invalid run identity")
    if command("git", "rev-parse", "HEAD") != source:
        raise ValueError("checkout does not match this run source SHA")
    with open(os.environ["GITHUB_EVENT_PATH"], encoding="utf-8") as stream:
        event = json.load(stream)
    repo = os.environ["GITHUB_REPOSITORY"]
    if event.get("repository", {}).get("full_name") != repo:
        raise ValueError("event repository mismatch")
    number = None
    base = None
    head = source
    full = False
    if event_name == "pull_request":
        pr = event["pull_request"]
        number = pr["number"]
        if pr["base"]["ref"] != "main" or pr["base"]["repo"]["full_name"] != repo:
            raise ValueError("PR must target this repository main")
        base = sha(pr["base"]["sha"])
        head = sha(pr["head"]["sha"])
        paths = changed_files(base, head, merge_base=True)
    elif event_name == "push":
        if ref != "refs/heads/main" or event.get("deleted") or sha(event["after"]) != source:
            raise ValueError("only a main push can produce full CI evidence")
        full = True
        before = sha(event["before"])
        paths = ["Makefile"] if before == "0" * 40 else changed_files(before, source)
        base = before
    elif event_name == "workflow_dispatch":
        if ref == "refs/heads/main":
            full = True
            paths = ["Makefile"]
        else:
            from release_common import release_branch, trusted_release_pr
            components = [component for component in ("kite", "player") if ref == "refs/heads/" + release_branch(component)]
            if len(components) != 1:
                raise ValueError("dispatch ref is not main or a trusted release branch")
            component = components[0]
            prs = github_api(f"pulls?state=open&base=main&head={repo.split('/')[0]}:{release_branch(component)}&per_page=100")
            matches = [pr for pr in prs if trusted_release_pr(pr, component) and pr.get("state") == "open" and pr["head"]["sha"] == source]
            if len(matches) != 1:
                raise ValueError("dispatch has no unique trusted release PR at this exact SHA")
            pr = matches[0]
            number = pr["number"]
            base = sha(pr["base"]["sha"])
            paths = changed_files(base, source, merge_base=True)
    else:
        raise ValueError("unsupported CI event")
    if number is not None and (type(number) is not int or number <= 0):
        raise ValueError("invalid PR number")
    return {
        "event": event_name, "repository": repo, "ref": ref, "sourceSha": source,
        "runId": run_id, "runAttempt": int(attempt), "prNumber": number,
        "baseSha": base, "headSha": head, "full": full, "paths": paths,
    }


def plan():
    origin = context()
    selected = selected_tasks(origin["paths"], origin["full"])
    modules = dependency_modules(origin["paths"])
    if modules:
        selected.add("security")
    if origin["prNumber"] is not None:
        selected.add("metadata")
    return {
        "schemaVersion": 1, "context": origin,
        "tasks": {name: name in selected for name in TASKS},
        "securityModules": modules,
    }


def no_duplicate_keys(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def decode(value):
    return json.loads(value, object_pairs_hook=no_duplicate_keys)


def gate(needs):
    if not isinstance(needs, dict) or set(needs) != {"plan", *TASKS}:
        raise ValueError("missing or unregistered CI task")
    if needs["plan"].get("result") != "success":
        raise ValueError("CI plan did not succeed")
    actual = decode(needs["plan"]["outputs"]["plan"])
    expected = plan()
    # Recompute from the event and the full Git diff, not untrusted task flags.
    # Comparing canonical JSON also rejects bool/int coercion and extra fields.
    if json.dumps(actual, sort_keys=True) != json.dumps(expected, sort_keys=True):
        raise ValueError("CI plan is malformed, forged, stale, or belongs to another run attempt")
    for task, required in expected["tasks"].items():
        result = needs[task].get("result")
        if result != "success" and not (result == "skipped" and not required):
            raise ValueError(f"{task}: {result!r}; required={required}")
        print(f"{task}: {result}")


def main():
    if len(sys.argv) != 2 or sys.argv[1] not in ("plan", "check"):
        raise ValueError("usage: ci-plan.py plan|check")
    validate_workflow()
    if sys.argv[1] == "check":
        gate(decode(sys.stdin.read()))
        return
    value = plan()
    encoded = json.dumps(value, separators=(",", ":"))
    print(encoded)
    if os.environ.get("GITHUB_OUTPUT"):
        with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as stream:
            stream.write(f"plan={encoded}\n")
            stream.write("tasks=" + json.dumps(value["tasks"], separators=(",", ":")) + "\n")
            stream.write("security_modules=" + " ".join(value["securityModules"]) + "\n")
            stream.write("pr_number=" + str(value["context"]["prNumber"] or "") + "\n")


if __name__ == "__main__":
    try:
        main()
    except (KeyError, TypeError, ValueError, OSError, subprocess.CalledProcessError, yaml.YAMLError) as error:
        print(f"::error::{error}", file=sys.stderr)
        sys.exit(1)
