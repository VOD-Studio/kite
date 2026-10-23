#!/usr/bin/env python3
"""Select a public Go release; automatic deployments never roll back a newer release."""
import os
import subprocess
import sys

from release_common import parse_tag, release_for, releases, require_main, resolve_source


def select(tag, automatic):
    require_main()
    version = tuple(map(int, parse_tag(tag, "kite").split(".")))
    if not automatic and os.environ.get("GITHUB_EVENT_NAME") != "workflow_dispatch":
        raise ValueError("旧版本只能通过 main 上的手动 Deploy 显式部署")
    release = release_for(tag)
    if release.get("draft") is not False or release.get("prerelease") is not False:
        raise ValueError("部署只接受已公开的 Go 正式版本")
    commit = resolve_source(tag)
    if automatic:
        public_versions = set()
        for item in releases():
            if item.get("draft") is not False or item.get("prerelease") is not False:
                continue
            try:
                candidate = parse_tag(item.get("tag_name"), "kite")
            except ValueError:
                continue
            public_versions.add(tuple(map(int, candidate.split("."))))
        if version not in public_versions:
            raise ValueError("校验期间公开版本列表发生变化;请重试")
        if version != max(public_versions):
            print(f"::notice::{tag} 不是最新 Go 正式版;不自动回退生产服务", file=sys.stderr)
            return commit["sha"], False
    return commit["sha"], True


def main():
    automatic = os.environ["AUTOMATIC"]
    if automatic not in ("true", "false"):
        raise ValueError("AUTOMATIC 必须明确为 true 或 false")
    sha, deploy = select(os.environ["TAG"], automatic == "true")
    print(f"sha={sha}")
    print(f"deploy={str(deploy).lower()}")


if __name__ == "__main__":
    try:
        main()
    except (KeyError, ValueError, subprocess.CalledProcessError) as error:
        print(f"::error::{error}", file=sys.stderr)
        sys.exit(1)
