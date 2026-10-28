#!/usr/bin/env python3
"""Publish only the player tarball already consumed by the trusted main CI run."""
import base64
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import subprocess
import sys
import tarfile
import time
from urllib.error import HTTPError
from urllib.parse import quote
from urllib.request import urlopen
import zipfile

import release_common as common

NAME = "@violetjs/kite-player"
REGISTRY = "https://registry.npmjs.org"
FILES = {"kite-player.tgz", "manifest.json"}
FIELDS = {"schemaVersion", "name", "version", "sourceSha", "runId", "runAttempt", "tarball", "sha256", "integrity"}


def positive(value):
    if type(value) is not int or value < 1:
        raise ValueError("missing or invalid run/release identity")
    return value


def context():
    common.require_main()
    tag = os.environ["TAG"]
    value = common.parse_tag(tag, "player")
    release = common.release_for(tag)
    if release.get("tag_name") != tag or type(release.get("draft")) is not bool or release.get("prerelease") is not ("-" in value):
        raise ValueError("release identity or prerelease flag does not match player tag")
    positive(release["id"])
    commit = common.resolve_source(tag)
    sha = commit["sha"]
    if not re.fullmatch(r"[0-9a-f]{40}", sha):
        raise ValueError("invalid source SHA")
    common.require_release_pr(commit, "player", value)
    run = common.wait_main_ci(sha)
    positive(run["id"])
    positive(run["run_attempt"])
    if run.get("head_sha") != sha:
        raise ValueError("CI producer SHA differs from release source")
    return value, release, sha, run


def directory():
    return Path(os.environ["RUNNER_TEMP"]) / "kite-player-release"


def download(run, sha):
    pages = json.loads(common.command("gh", "api", "--paginate", "--slurp", f"repos/{common.REPO}/actions/runs/{run['id']}/artifacts?per_page=100"))
    name = f"kite-player-package-{run['run_attempt']}"
    matches = [artifact for page in pages for artifact in page["artifacts"] if artifact["name"] == name]
    if len(matches) != 1:
        raise ValueError("expected one package artifact for the successful CI run attempt")
    artifact = matches[0]
    producer = artifact.get("workflow_run", {})
    if artifact.get("expired") is not False or producer.get("id") != run["id"] or producer.get("head_sha") != sha:
        raise ValueError("artifact is expired or belongs to another source/run")
    positive(artifact["id"])
    archive = subprocess.check_output(("gh", "api", f"repos/{common.REPO}/actions/artifacts/{artifact['id']}/zip"))
    with zipfile.ZipFile(io.BytesIO(archive)) as package:
        members = package.infolist()
        if len(members) != len(FILES) or {member.filename for member in members} != FILES:
            raise ValueError("artifact must contain only the two fixed safe filenames")
        for member in members:
            mode = member.external_attr >> 16
            if member.is_dir() or stat.S_IFMT(mode) not in (0, stat.S_IFREG):
                raise ValueError("artifact files must be regular files")
        root = directory()
        root.mkdir(mode=0o700, parents=True, exist_ok=False)
        for member in members:
            (root / member.filename).write_bytes(package.read(member))


def hashes(data):
    return hashlib.sha256(data).hexdigest(), "sha512-" + base64.b64encode(hashlib.sha512(data).digest()).decode("ascii")


def verified(value, sha, run):
    root = directory()
    if root.is_symlink() or not root.is_dir() or {item.name for item in root.iterdir()} != FILES:
        raise ValueError("unexpected artifact directory contents")
    for filename in FILES:
        item = root / filename
        if item.is_symlink() or not item.is_file():
            raise ValueError("artifact paths must be regular files")
    manifest = json.loads((root / "manifest.json").read_text(encoding="utf-8"))
    if not isinstance(manifest, dict) or set(manifest) != FIELDS:
        raise ValueError("manifest schema fields differ from the supported contract")
    expected = {"schemaVersion": 1, "name": NAME, "version": value, "sourceSha": sha,
                "runId": str(run["id"]), "runAttempt": run["run_attempt"], "tarball": "kite-player.tgz"}
    if any(type(manifest[key]) is not type(wanted) or manifest[key] != wanted for key, wanted in expected.items()):
        raise ValueError("manifest source, version or CI attempt does not match authorization")
    data = (root / "kite-player.tgz").read_bytes()
    digest, integrity = hashes(data)
    if manifest["sha256"] != digest or manifest["integrity"] != integrity:
        raise ValueError("tarball hash differs from tested manifest")
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as package:
        members = package.getmembers()
        seen = set()
        metadata = None
        for member in members:
            path = PurePosixPath(member.name)
            if (path.is_absolute() or ".." in path.parts or "\\" in member.name
                    or not path.parts or path.parts[0] != "package"
                    or str(path) != member.name.rstrip("/") or member.name in seen
                    or not (member.isfile() or member.isdir())):
                raise ValueError("unsafe or duplicate tarball member")
            seen.add(member.name)
            if member.name == "package/package.json":
                if not member.isfile():
                    raise ValueError("package.json must be a regular file")
                metadata = json.load(package.extractfile(member))
        if not isinstance(metadata, dict) or metadata.get("name") != NAME or metadata.get("version") != value:
            raise ValueError("tarball package.json does not match player name/version")
    return data, integrity


def fetch(url, missing=False):
    try:
        with urlopen(url, timeout=60) as response:
            if response.geturl() != url:
                raise ValueError("unexpected registry redirect")
            return response.read()
    except HTTPError as error:
        if missing and error.code == 404:
            return None
        raise ValueError(f"registry request failed with HTTP {error.code}") from error


def published(value, data, integrity):
    metadata = fetch(f"{REGISTRY}/{quote(NAME, safe='')}/{quote(value, safe='')}", missing=True)
    if metadata is None:
        return False
    metadata = json.loads(metadata)
    if not isinstance(metadata, dict) or metadata.get("name") != NAME or metadata.get("version") != value:
        raise ValueError("registry package identity does not match the release")
    dist = metadata.get("dist", {})
    expected_url = f"{REGISTRY}/{NAME}/-/kite-player-{value}.tgz"
    if dist.get("tarball") != expected_url or dist.get("integrity") != integrity:
        raise ValueError("existing npm version differs from the verified CI tarball")
    if fetch(expected_url) != data:
        raise ValueError("existing npm version has different bytes; immutable version cannot be overwritten")
    return True


def require_provenance(sha):
    if os.environ.get("GITHUB_SHA") != sha:
        raise ValueError("new publication requires provenance source SHA to equal the release source; rerun the original release-please workflow at that SHA, not a current-main manual recovery")


def output(key, value):
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as stream:
        stream.write(f"{key}={value}\n")


def prepare():
    value, _, sha, run = context()
    download(run, sha)
    data, integrity = verified(value, sha, run)
    exists = published(value, data, integrity)
    if not exists:
        require_provenance(sha)
    output("publish", str(not exists).lower())


def publish():
    value, _, sha, run = context()
    data, integrity = verified(value, sha, run)
    if published(value, data, integrity):
        return
    require_provenance(sha)
    if not os.environ.get("NODE_AUTH_TOKEN"):
        raise ValueError("NPM_TOKEN is required only for new publication")
    subprocess.run(("npm", "publish", str(directory() / "kite-player.tgz"), "--registry", REGISTRY,
                    "--access", "public", "--provenance", "--ignore-scripts", "--tag",
                    "next" if "-" in value else "latest"), check=True)


def wait_published(value, data, integrity):
    # npm acknowledges a publish before the version is readable from the registry.
    # Only a missing version (404) is retried; every other registry answer is final.
    tries = int(os.environ.get("NPM_POLL_TRIES", "20"))
    for attempt in range(tries):
        if published(value, data, integrity):
            return True
        if attempt + 1 < tries:
            time.sleep(int(os.environ.get("NPM_POLL_SLEEP", "15")))
    return False


def finalize():
    value, release, sha, run = context()
    data, integrity = verified(value, sha, run)
    if not wait_published(value, data, integrity):
        raise ValueError("npm version is not publicly available; GitHub release remains a draft")
    if release["draft"]:
        common.api(f"releases/{release['id']}", "--method", "PATCH", "-F", "draft=false", "-f", "make_latest=false")


if __name__ == "__main__":
    try:
        if len(sys.argv) != 2 or sys.argv[1] not in ("prepare", "publish", "finalize"):
            raise ValueError("usage: npm-release.py prepare|publish|finalize")
        {"prepare": prepare, "publish": publish, "finalize": finalize}[sys.argv[1]]()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError, tarfile.TarError, zipfile.BadZipFile) as error:
        sys.exit(f"npm release refused: {error}")
