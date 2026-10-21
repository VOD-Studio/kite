#!/usr/bin/env python3
"""Trusted-main release gate. No build bypass and no writes to public assets."""
import json
import os
import re
import subprocess
import sys
import time

REPO = "VOD-Studio/kite"
IMAGE = "ghcr.io/vod-studio/kite-server"
TAG_RE = re.compile(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\Z")


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def api(path, *args):
    return json.loads(command("gh", "api", f"repos/{REPO}/{path}", *args))


def version(tag):
    match = TAG_RE.fullmatch(tag)
    if not match:
        raise ValueError("tag must be canonical vX.Y.Z")
    return tuple(map(int, match.groups()))


def output(key, value):
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as stream:
        stream.write(f"{key}={value}\n")


def context():
    if os.environ.get("GITHUB_REPOSITORY") != REPO or os.environ.get("GITHUB_REF") != "refs/heads/main":
        raise ValueError("release tooling must run from VOD-Studio/kite main")
    tag = os.environ["TAG"]
    version(tag)
    release = release_for(tag)
    if release["prerelease"]:
        raise ValueError("not a stable release")
    commit = api(f"commits/{tag}")
    comparison = api(f"compare/{commit['sha']}...main")
    if comparison["status"] not in ("ahead", "identical"):
        raise ValueError("release source is not an ancestor of main")
    return tag, release, commit


def prepare():
    tag, release, commit = context()
    output("sha", commit["sha"])
    # Public history is verification-only, including old tags without the new gate.
    # Never invoke GoReleaser or upload/replace assets on this path.
    if not release["draft"]:
        output("build", "false")
        return
    if len(commit["parents"]) != 1:
        raise ValueError("release PR must be squash merged (release commit requires exactly one parent)")
    prs = api(f"commits/{commit['sha']}/pulls")
    trusted = [pr for pr in prs if pr.get("merged_at") and pr["merge_commit_sha"] == commit["sha"]
               and pr["user"]["login"] == "github-actions[bot]" and pr["user"]["type"] == "Bot"
               and pr["base"]["ref"] == "main" and pr["base"]["repo"]["full_name"] == REPO
               and pr["head"]["repo"] and pr["head"]["repo"]["full_name"] == REPO
               and pr["head"]["ref"] == "release-please--branches--main"
               and pr["title"] == f"chore(main): release {tag[1:]}"]
    if len(trusted) != 1:
        raise ValueError("tag is not the squash commit of one trusted release PR")
    workflow = api("actions/workflows/ci.yml")["id"]
    for attempt in range(int(os.environ.get("CI_POLL_TRIES", "50"))):
        runs = api(f"actions/workflows/{workflow}/runs?event=push&branch=main&head_sha={commit['sha']}&per_page=100")["workflow_runs"]
        runs = [r for r in runs if r["workflow_id"] == workflow and r["event"] == "push"
                and r["head_sha"] == commit["sha"] and r["head_branch"] == "main"
                and r["head_repository"]["full_name"] == REPO]
        if runs:
            run = max(runs, key=lambda r: (r["run_number"], r["run_attempt"]))
            if run["status"] == "completed":
                if run["conclusion"] != "success":
                    raise ValueError("the corresponding main ci.yml producer failed")
                jobs = json.loads(command("gh", "api", "--paginate", "--slurp", f"repos/{REPO}/actions/runs/{run['id']}/attempts/{run['run_attempt']}/jobs?per_page=100"))
                gates = [j for page in jobs for j in page["jobs"] if j["name"] == "CI OK"]
                if len(gates) != 1 or gates[0]["conclusion"] != "success":
                    raise ValueError("ci.yml did not produce one successful CI OK gate")
                output("build", "true")
                return
        if attempt + 1 < int(os.environ.get("CI_POLL_TRIES", "50")):
            time.sleep(int(os.environ.get("CI_POLL_SLEEP", "30")))
    raise ValueError("timed out waiting for the release commit's main ci.yml run")


def releases():
    pages = json.loads(command("gh", "api", "--paginate", "--slurp", f"repos/{REPO}/releases?per_page=100"))
    return [r for page in pages for r in page]


def release_for(tag):
    # releases/tags/{tag} never returns drafts (404), and every new release starts as a
    # draft, so look it up in the authenticated listing instead.
    matches = [r for r in releases() if r["tag_name"] == tag]
    if len(matches) != 1:
        raise ValueError(f"expected exactly one release for {tag}, found {len(matches)}")
    return matches[0]


def public_versions():
    return [r["tag_name"] for r in releases() if not r["draft"] and not r["prerelease"] and TAG_RE.fullmatch(r["tag_name"])]


def publish():
    tag, release, commit = context()
    if commit["sha"] != os.environ["SOURCE_SHA"]:
        raise ValueError("tag moved during publication")
    versions = public_versions()
    latest = max(versions + [tag], key=version) == tag
    if release["draft"]:
        # Called only after downloaded assets, signatures and the pushed image pass.
        api(f"releases/{release['id']}", "--method", "PATCH", "-F", "draft=false", "-f", f"make_latest={str(latest).lower()}")
    # Replays repair floating tags without changing immutable version assets.
    # Global queue:max serializes publish + these promotions across versions.
    versions = public_versions()
    if max(versions, key=version) == tag:
        command("docker", "buildx", "imagetools", "create", "--tag", f"{IMAGE}:latest", os.environ["VERIFIED_IMAGE"])
    minor = version(tag)[:2]
    if max((v for v in versions if version(v)[:2] == minor), key=version) == tag:
        command("docker", "buildx", "imagetools", "create", "--tag", f"{IMAGE}:v{minor[0]}.{minor[1]}", os.environ["VERIFIED_IMAGE"])


if __name__ == "__main__":
    try:
        {"prepare": prepare, "publish": publish}[sys.argv[1]]()
    except (ValueError, subprocess.CalledProcessError) as error:
        sys.exit(f"release refused: {error}")
