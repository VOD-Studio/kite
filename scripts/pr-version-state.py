#!/usr/bin/env python3
"""Validate release state in the real three-way merge, not the PR head snapshot."""
import json
import re
import subprocess
import sys

import release_common as common

MANIFEST = ".release-please-manifest.json"
PRODUCTS = {"kite": ".", "player": "packages/kite-player"}
VERSION = re.compile(rf"{common.SEMVER}(?:-({common.PRERELEASE}))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?\Z")


def precedence(value):
    match = VERSION.fullmatch(value) if isinstance(value, str) else None
    if not match:
        raise ValueError(f"invalid manifest version: {value!r}")
    major, minor, patch, prerelease = match.groups()
    # SemVer: numeric identifiers precede strings; a stable release follows all prereleases.
    identifiers = tuple((0, int(part)) if part.isdecimal() else (1, part)
                        for part in prerelease.split(".")) if prerelease else ()
    return int(major), int(minor), int(patch), prerelease is None, identifiers


def manifest(revision):
    raw = subprocess.check_output(["git", "show", f"{revision}:{MANIFEST}"], text=True)
    state = json.loads(raw)
    if not isinstance(state, dict) or not all(isinstance(key, str) for key in state):
        raise ValueError("release manifest must be a product/version object")
    for value in state.values():
        precedence(value)
    for component, path in PRODUCTS.items():
        if path not in state:
            raise ValueError(f"release manifest is missing {component}")
        common.parse_tag(("v" if component == "kite" else "player-v") + state[path], component)
    return state


def validate(pr, base, head):
    for revision in (base, head):
        if not common.SHA.fullmatch(revision):
            raise ValueError("version guard requires full base/head commit SHAs")
    if pr.get("head", {}).get("sha") != head:
        raise ValueError("PR head changed since the CI plan; rerun CI")
    # merge-tree uses the actual merge base and never modifies the checkout/index.
    merged = subprocess.run(["git", "merge-tree", "--write-tree", base, head],
                            text=True, capture_output=True, check=False)
    if merged.returncode:
        raise ValueError("cannot validate version state: PR does not merge cleanly")
    tree = merged.stdout.splitlines()[0]
    before, after = manifest(base), manifest(tree)
    for product, version in before.items():
        if product not in after:
            raise ValueError(f"release manifest deleted product {product}")
        if precedence(after[product]) < precedence(version):
            raise ValueError(f"release manifest regressed {product}: {version} -> {after[product]}")
    for component, product in PRODUCTS.items():
        if pr.get("head", {}).get("ref") != common.release_branch(component):
            continue
        if precedence(after[product]) <= precedence(before[product]):
            raise ValueError(f"{component} release PR must increase its own version")
        if {key: value for key, value in before.items() if key != product} != {
                key: value for key, value in after.items() if key != product}:
            raise ValueError(f"{component} release PR changed another product version")
        if not common.trusted_release_pr(pr, component, after[product]):
            raise ValueError(f"{component} release identity/title does not match the merged version")


def main():
    try:
        if len(sys.argv) != 3:
            raise ValueError("usage: pr-version-state.py BASE_SHA HEAD_SHA (PR JSON on stdin)")
        validate(json.load(sys.stdin), sys.argv[1], sys.argv[2])
    except (ValueError, subprocess.CalledProcessError) as error:
        print(f"::error::{error}", file=sys.stderr)
        return 1
    print("PR merged release versions preserve product state")
    return 0


if __name__ == "__main__":
    sys.exit(main())
