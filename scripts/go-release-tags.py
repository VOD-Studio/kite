#!/usr/bin/env python3
"""Select canonical Go tags for OSS GoReleaser, without changing any Git refs."""
import argparse
import subprocess
import sys

from release_common import parse_tag


def git(*args):
    return subprocess.check_output(("git", *args), text=True).strip()


def version(tag):
    return tuple(map(int, parse_tag(tag, "kite").split(".")))


def reachable_tags(ref):
    tags = []
    for tag in git("tag", "--merged", ref).splitlines():
        try:
            version(tag)
        except ValueError:
            continue
        tags.append(tag)
    return tags


def select(tag=None):
    head = git("rev-parse", "HEAD")
    if tag is not None:
        version(tag)
        source = git("rev-parse", "--verify", f"refs/tags/{tag}^{{commit}}")
        if source != head:
            raise ValueError("formal Go release tag must point at the checked-out source")
        current = tag
    else:
        # Snapshot without any Go release uses GoReleaser's own initial-version convention.
        tags = reachable_tags(head)
        current = max(tags, key=version, default="v0.0.0")
        source = git("rev-parse", f"refs/tags/{current}^{{commit}}") if tags else head
    parents = git("rev-list", "--parents", "-n", "1", source).split()[1:]
    older = {candidate for parent in parents for candidate in reachable_tags(parent) if version(candidate) < version(current)}
    # An empty environment value asks GoReleaser to auto-detect again (and can
    # select a player tag). Its getFromEnv accepts a commit SHA as PreviousTag;
    # use the root commit when no earlier Go version exists, not a fabricated tag.
    previous = max(older, key=version) if older else sorted(git("rev-list", "--max-parents=0", source).splitlines())[0]
    return current, previous


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag", nargs="?", help="Exact formal Go tag; omit for a snapshot")
    args = parser.parse_args()
    try:
        current, previous = select(args.tag)
        print(f"GORELEASER_CURRENT_TAG={current}")
        print(f"GORELEASER_PREVIOUS_TAG={previous}")
    except (ValueError, subprocess.CalledProcessError) as error:
        sys.exit(f"Go tag selection refused: {error}")


if __name__ == "__main__":
    main()
