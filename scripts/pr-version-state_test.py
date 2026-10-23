#!/usr/bin/env python3
"""PR Metadata behavior against real Git histories and mocked GitHub responses."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import release_common as common

SCRIPT = Path(__file__).with_name("pr-commits.sh")
MANIFEST = ".release-please-manifest.json"


class VersionStateTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.repo = self.root / "repo"
        self.repo.mkdir()
        self.git("init", "-q", "-b", "main")
        self.git("config", "user.name", "Version state test")
        self.git("config", "user.email", "version-test@example.invalid")
        self.old = self.commit("0.7.0", "0.1.0")
        self.base = self.commit("0.7.1", "0.1.0")
        self.gh = self.root / "gh"
        self.gh.write_text('#!/bin/sh\ncase "$*" in\n'
                           '  *"/commits?"*) cat "$CASE_COMMITS" ;;\n'
                           '  *"/pulls/42") cat "$CASE_PR" ;;\n'
                           '  *) exit 90 ;;\nesac\n')
        self.gh.chmod(0o755)
        self.commits = self.root / "commits.json"
        self.commits.write_text(json.dumps([{
            "sha": "a", "commit": {"message": "chore: update versions"}, "parents": [{}],
        }]))

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.repo, text=True,
                                       stderr=subprocess.PIPE).strip()

    def commit(self, kite, player):
        # Separation lets independently edited product lines merge cleanly; conflicts
        # themselves are tested separately, rather than bypassed with a fake merge.
        (self.repo / MANIFEST).write_text(
            '{\n  ".": ' + json.dumps(kite) + ',\n\n  "packages/kite-player": '
            + json.dumps(player) + '\n}\n')
        self.git("add", ".")
        self.git("commit", "-qm", "chore: update versions")
        return self.git("rev-parse", "HEAD")

    def branch(self, start=None):
        self.git("checkout", "-qb", "topic", start or self.base)

    def check(self, head, component=None, version=None, error=None):
        pr = {
            "base": {"ref": "main", "sha": self.base, "repo": {"full_name": common.REPO}},
            "head": {"ref": common.release_branch(component) if component else "feat/topic",
                     "sha": head, "repo": {"full_name": common.REPO}},
            "user": {"login": "github-actions[bot]" if component else "contributor",
                     "type": "Bot" if component else "User"},
            "title": f"chore({component}): release {version}" if component else "fix: preserve version state",
            "state": "open", "commits": 1,
        }
        fixture = self.root / "pr.json"
        fixture.write_text(json.dumps(pr))
        result = subprocess.run(["bash", str(SCRIPT)], cwd=self.repo, text=True,
                                capture_output=True, check=False, env={
                                    **os.environ, "GH": str(self.gh), "GH_REPO": common.REPO,
                                    "PR_NUMBER": "42", "PR_BASE_SHA": self.base, "PR_HEAD_SHA": head,
                                    "CASE_PR": str(fixture), "CASE_COMMITS": str(self.commits),
                                })
        output = result.stdout + result.stderr
        self.assertEqual(result.returncode, 1 if error else 0, output)
        if error:
            self.assertIn(error, output)

    def test_player_125_resolved_merge_regresses_go(self):
        self.branch(self.old)
        self.commit("0.7.0", "0.2.0")
        # Reproduce a release branch updated from main but resolved with stale Go state.
        self.git("merge", "--no-commit", "--no-ff", self.base)
        head = self.commit("0.7.0", "0.2.0")
        self.check(head, "player", "0.2.0", error="regressed .: 0.7.1 -> 0.7.0")

    def test_ordinary_pr_cannot_regress_go(self):
        self.branch()
        self.check(self.commit("0.7.0", "0.1.0"), error="regressed .")

    def test_healthy_go_release(self):
        self.branch()
        self.check(self.commit("0.7.2", "0.1.0"), "kite", "0.7.2")

    def test_healthy_player_release(self):
        self.branch()
        self.check(self.commit("0.7.1", "0.2.0"), "player", "0.2.0")

    def test_updated_player_branch_preserves_main_go_version(self):
        self.branch(self.old)
        self.commit("0.7.0", "0.2.0")
        self.git("merge", "--no-edit", "--no-ff", self.base)
        self.check(self.git("rev-parse", "HEAD"), "player", "0.2.0")

    def test_old_player_branch_uses_three_way_merge_not_head_snapshot(self):
        self.branch(self.old)
        self.check(self.commit("0.7.0", "0.2.0"), "player", "0.2.0")

    def test_old_ordinary_branch_does_not_regress_untouched_manifest(self):
        self.branch(self.old)
        (self.repo / "feature.txt").write_text("feature\n")
        self.git("add", ".")
        self.git("commit", "-qm", "feat: feature")
        self.check(self.git("rev-parse", "HEAD"))

    def test_release_cannot_increase_other_product(self):
        self.branch()
        head = self.commit("0.7.2", "0.2.0")
        for component, version in (("kite", "0.7.2"), ("player", "0.2.0")):
            with self.subTest(component=component):
                self.check(head, component, version, error="changed another product version")

    def test_release_must_increase_own_version(self):
        self.check(self.base, "player", "0.1.0", error="must increase its own version")

    def test_release_title_must_match_merged_version(self):
        self.branch()
        self.check(self.commit("0.7.1", "0.2.0"), "player", "0.3.0",
                   error="does not match the merged version")

    def test_invalid_versions_are_rejected(self):
        self.branch()
        for version in ("01.2.0", "0.2.0-01", "not-a-version", None, 2):
            with self.subTest(version=version):
                self.check(self.commit("0.7.1", version), error="invalid manifest version")

    def test_deleted_product_is_rejected(self):
        self.branch()
        (self.repo / MANIFEST).write_text('{".": "0.7.1"}\n')
        self.git("commit", "-qam", "chore: remove product")
        self.check(self.git("rev-parse", "HEAD"), error="missing player")

    def test_deleted_manifest_is_rejected(self):
        self.branch()
        self.git("rm", MANIFEST)
        self.git("commit", "-qm", "chore: remove manifest")
        self.check(self.git("rev-parse", "HEAD"), error="::error::")

    def test_semver_prerelease_precedence(self):
        self.branch()
        versions = ("0.2.0-alpha", "0.2.0-alpha.1", "0.2.0-alpha.2", "0.2.0-alpha.10",
                    "0.2.0-beta", "0.2.0-rc.1", "0.2.0")
        for previous, following in zip(versions, versions[1:]):
            with self.subTest(previous=previous, following=following):
                self.base = self.commit("0.7.1", previous)
                head = self.commit("0.7.1", following)
                self.check(head, "player", following)
                self.base = head
                head = self.commit("0.7.1", previous)
                self.check(head, "player", previous, error="regressed packages/kite-player")

    def test_unresolved_merge_cannot_pass_metadata(self):
        self.branch(self.old)
        head = self.commit("0.7.2", "0.2.0")
        self.check(head, "player", "0.2.0", error="does not merge cleanly")


if __name__ == "__main__":
    unittest.main()
