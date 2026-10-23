#!/usr/bin/env python3
"""Route existing releases owned by this run's exact source, never action outputs.

release-please v17.6.0 scans merged pending PRs on the live target branch and
removes their pending labels after creating releases, even when that branch has
advanced past the calling run. See findMergedReleasePullRequests and
createReleasesForPullRequest in:
https://github.com/googleapis/release-please/blob/v17.6.0/src/manifest.ts
The commit-associated PR and actual tag, not those transient labels or the
release_created output, authorize routing. Publication remains the downstream
publisher's responsibility, including byte-for-byte public-version replay checks.
"""
import os
import subprocess
import sys

import release_common as common


def route():
    common.require_main()
    sha = os.environ.get("GITHUB_SHA", "")
    if not common.SHA.fullmatch(sha):
        raise ValueError("release routing requires this run's full GITHUB_SHA")

    # One bounded query for this source, not a commit lookup per historical release.
    # A full page might hide a second candidate, so reject instead of truncating.
    prs = common.api(f"commits/{sha}/pulls?per_page=100")
    if not isinstance(prs, list) or len(prs) >= 100:
        raise ValueError("source PR query is invalid or exceeds the routing bound")
    branches = {common.release_branch(component) for component in common.COMPONENTS}
    title_prefixes = tuple(f"chore({component}): release " for component in common.COMPONENTS)
    title_prefixes += ("chore(main): release ",)
    candidates = [pr for pr in prs if pr.get("head", {}).get("ref") in branches
                  or pr.get("title", "").startswith(title_prefixes)]
    outputs = {"go_tag": "", "player_tag": ""}
    if not candidates:
        return outputs
    if len(candidates) != 1:
        raise ValueError("source must identify exactly one component release PR")
    pr = candidates[0]
    components = [component for component in common.COMPONENTS
                  if common.trusted_merged_release_pr(pr, component)]
    if (len(components) != 1 or pr.get("state") != "closed"
            or not pr.get("merged_at") or pr.get("merge_commit_sha") != sha):
        raise ValueError("source is not the merged commit of one trusted component release PR")
    component = components[0]
    version = pr["title"].rsplit(" ", 1)[1]
    tag = ("v" if component == "kite" else "player-v") + version
    common.parse_tag(tag, component)
    common.release_for(tag)  # Authenticated listing includes already-created drafts.
    commit = common.resolve_source(tag)  # Also requires ancestry of current main.
    if commit["sha"] != sha:
        raise ValueError("release tag does not belong to this run's exact GITHUB_SHA")
    if len(commit.get("parents", [])) != 1:
        raise ValueError("release PR must produce one squash commit with exactly one parent")
    outputs["go_tag" if component == "kite" else "player_tag"] = tag
    return outputs


def main():
    outputs = route()
    # No outputs are emitted until every authorization check has completed.
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
        for key, value in outputs.items():
            output.write(f"{key}={value}\n")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        print(f"release routing failed: {error}", file=sys.stderr)
        sys.exit(1)
