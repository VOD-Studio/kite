#!/usr/bin/env python3
"""Trusted-main release gate. No build bypass and no writes to public assets."""
import os
import re
import subprocess
import sys

import release_common as common

REPO = common.REPO
IMAGE = "ghcr.io/vod-studio/kite-server"
TAG_RE = re.compile(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\Z")


def version(tag):
    return tuple(map(int, common.parse_tag(tag, "kite").split(".")))


def output(key, value):
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as stream:
        stream.write(f"{key}={value}\n")


def context():
    common.require_main()
    tag = os.environ["TAG"]
    version(tag)
    release = common.release_for(tag)
    if release["prerelease"]:
        raise ValueError("not a stable release")
    commit = common.resolve_source(tag)
    return tag, release, commit


def prepare():
    tag, release, commit = context()
    output("sha", commit["sha"])
    # Public history is verification-only, including old tags without the new gate.
    # Never invoke GoReleaser or upload/replace assets on this path.
    if not release["draft"]:
        output("build", "false")
        return
    common.require_release_pr(commit, "kite", tag[1:])
    common.wait_main_ci(commit["sha"])
    output("build", "true")


def public_versions():
    return [r["tag_name"] for r in common.releases() if not r["draft"] and not r["prerelease"] and TAG_RE.fullmatch(r["tag_name"])]


def publish():
    tag, release, commit = context()
    if commit["sha"] != os.environ["SOURCE_SHA"]:
        raise ValueError("tag moved during publication")
    if release["draft"]:
        common.require_release_pr(commit, "kite", tag[1:])
        common.wait_main_ci(commit["sha"])
        versions = public_versions()
        latest = max(versions + [tag], key=version) == tag
        # Called only after downloaded assets, signatures and the pushed image pass.
        common.api(f"releases/{release['id']}", "--method", "PATCH", "-F", "draft=false", "-f", f"make_latest={str(latest).lower()}")
    # Public replays never replace version assets or patch a public release.
    # They may finish interrupted, monotonic promotions of the verified digest.
    # Global queue:max serializes publication and floating promotions.
    versions = public_versions()
    if max(versions, key=version) == tag:
        common.command("docker", "buildx", "imagetools", "create", "--tag", f"{IMAGE}:latest", os.environ["VERIFIED_IMAGE"])
    minor = version(tag)[:2]
    if max((v for v in versions if version(v)[:2] == minor), key=version) == tag:
        common.command("docker", "buildx", "imagetools", "create", "--tag", f"{IMAGE}:v{minor[0]}.{minor[1]}", os.environ["VERIFIED_IMAGE"])


if __name__ == "__main__":
    try:
        {"prepare": prepare, "publish": publish}[sys.argv[1]]()
    except (ValueError, subprocess.CalledProcessError) as error:
        sys.exit(f"release refused: {error}")
