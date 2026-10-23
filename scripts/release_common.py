#!/usr/bin/env python3
"""Shared release identity and exact main-push CI authorization; no remote writes."""
import json
import os
import re
import subprocess
import time

REPO = "VOD-Studio/kite"
SEMVER = r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
PRERELEASE = r"(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*"
SHA = re.compile(r"[0-9a-f]{40}\Z")
COMPONENTS = ("kite", "player")


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def api(path, *args):
    return json.loads(command(os.environ.get("GH", "gh"), "api", f"repos/{REPO}/{path}", *args))


def pages(path):
    return json.loads(command(os.environ.get("GH", "gh"), "api", "--paginate", "--slurp", f"repos/{REPO}/{path}"))


def parse_tag(tag, component):
    if component not in COMPONENTS:
        raise ValueError("unknown release component")
    prefix = "v" if component == "kite" else "player-v"
    suffix = "" if component == "kite" else rf"(?:-{PRERELEASE})?"
    if not isinstance(tag, str) or not re.fullmatch(re.escape(prefix) + SEMVER + suffix, tag):
        raise ValueError(f"invalid canonical {component} release tag")
    return tag[len(prefix):]


def release_branch(component):
    if component not in COMPONENTS:
        raise ValueError("unknown release component")
    return f"release-please--branches--main--components--{component}"


def trusted_release_pr(pr, component, version=None):
    """Check both open and merged PR identity; callers enforce state and exact SHA."""
    branch = release_branch(component)
    prefix = "chore(main): release " + ("player " if component == "player" else "")
    title = pr.get("title", "")
    if not title.startswith(prefix):
        return False
    candidate = title[len(prefix):]
    try:
        parse_tag(("v" if component == "kite" else "player-v") + candidate, component)
    except ValueError:
        return False
    return (
        (version is None or candidate == version)
        and pr.get("user", {}).get("login") == "github-actions[bot]"
        and pr.get("user", {}).get("type") == "Bot"
        and pr.get("base", {}).get("ref") == "main"
        and (pr.get("base", {}).get("repo") or {}).get("full_name") == REPO
        and (pr.get("head", {}).get("repo") or {}).get("full_name") == REPO
        and pr.get("head", {}).get("ref") == branch
    )


def require_main():
    workflows = ("release-please", "release", "npm", "deploy", "bsr")
    allowed = {f"{REPO}/.github/workflows/{name}.yml@refs/heads/main" for name in workflows}
    if (os.environ.get("GITHUB_REPOSITORY") != REPO
            or os.environ.get("GITHUB_REF") != "refs/heads/main"
            or os.environ.get("GITHUB_WORKFLOW_REF") not in allowed):
        raise ValueError("release tooling must run from a trusted VOD-Studio/kite main workflow")


def releases():
    return [release for page in pages("releases?per_page=100") for release in page]


def release_for(tag):
    # The by-tag endpoint hides drafts. Authenticated, paginated listing sees both.
    matches = [release for release in releases() if release["tag_name"] == tag]
    if len(matches) != 1:
        raise ValueError(f"expected exactly one release for {tag}, found {len(matches)}")
    return matches[0]


def resolve_source(tag):
    parse_tag(tag, "player" if tag.startswith("player-") else "kite")
    # A same-named branch must never substitute for the protected release tag.
    commit = api(f"commits/tags/{tag}")
    if not SHA.fullmatch(commit.get("sha", "")):
        raise ValueError("release source is not a full commit SHA")
    comparison = api(f"compare/{commit['sha']}...main")
    if comparison.get("status") not in ("ahead", "identical"):
        raise ValueError("release source is not an ancestor of main")
    return commit


def require_release_pr(commit, component, version):
    if not SHA.fullmatch(commit.get("sha", "")) or len(commit.get("parents", [])) != 1:
        raise ValueError("release PR must produce one squash commit with exactly one parent")
    prs = [pr for page in pages(f"commits/{commit['sha']}/pulls?per_page=100") for pr in page]
    trusted = [pr for pr in prs if pr.get("merged_at") and pr.get("merge_commit_sha") == commit["sha"]
               and trusted_release_pr(pr, component, version)]
    if len(trusted) != 1:
        raise ValueError("tag is not the squash commit of one trusted component release PR")


def main_ci_identity(run, workflow, sha):
    return (run.get("workflow_id") == workflow and run.get("event") == "push"
            and run.get("head_sha") == sha and run.get("head_branch") == "main"
            and (run.get("head_repository") or {}).get("full_name") == REPO
            and run.get("path") == ".github/workflows/ci.yml")


def wait_main_ci(sha):
    if not SHA.fullmatch(sha):
        raise ValueError("CI source must be a full commit SHA")
    workflow = api("actions/workflows/ci.yml")
    if workflow.get("path") != ".github/workflows/ci.yml" or workflow.get("state") != "active":
        raise ValueError("ci.yml is not the active trusted producer")
    workflow_id = workflow["id"]
    tries = int(os.environ.get("CI_POLL_TRIES", "50"))
    for attempt in range(tries):
        runs = [run for page in pages(f"actions/workflows/{workflow_id}/runs?event=push&branch=main&head_sha={sha}&per_page=100")
                for run in page["workflow_runs"] if main_ci_identity(run, workflow_id, sha)]
        if runs:
            candidate = max(runs, key=lambda run: (run["run_number"], run["run_attempt"]))
            # Listing can race a rerun. The authoritative run endpoint provides its latest attempt.
            run = api(f"actions/runs/{candidate['id']}")
            if not main_ci_identity(run, workflow_id, sha) or run.get("id") != candidate["id"]:
                raise ValueError("CI producer identity changed")
            if not isinstance(run.get("run_attempt"), int) or run["run_attempt"] < candidate["run_attempt"]:
                raise ValueError("CI producer attempt is stale or invalid")
            if run["status"] == "completed":
                if run.get("conclusion") != "success":
                    raise ValueError("the corresponding main ci.yml producer failed")
                jobs = [job for page in pages(f"actions/runs/{run['id']}/attempts/{run['run_attempt']}/jobs?per_page=100") for job in page["jobs"]]
                gates = [job for job in jobs if job.get("name") == "CI OK"]
                if len(gates) != 1 or gates[0].get("status") != "completed" or gates[0].get("conclusion") != "success":
                    raise ValueError("ci.yml did not produce one successful completed CI OK gate")
                latest = api(f"actions/runs/{run['id']}")
                if latest != run:
                    raise ValueError("CI producer changed while verifying its gate; retry publication")
                return run
            if run["status"] not in ("queued", "in_progress", "waiting", "pending", "requested"):
                raise ValueError("unknown CI producer status")
        if attempt + 1 < tries:
            time.sleep(int(os.environ.get("CI_POLL_SLEEP", "30")))
    raise ValueError("timed out waiting for the release commit's main ci.yml run")
