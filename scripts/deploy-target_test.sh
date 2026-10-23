#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
python3 - "$here" <<'PY'
import contextlib
import importlib.util
import io
import json
import os
import sys
import unittest
from unittest.mock import patch

sys.path.insert(0, sys.argv[1])
import release_common
spec = importlib.util.spec_from_file_location("deploy_target", os.path.join(sys.argv[1], "deploy-target.py"))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
sys.argv = [sys.argv[0]]

SHA = "a" * 40

def released(tag, **flags):
    return {"tag_name": tag, "draft": False, "prerelease": False, **flags}


class DeployTarget(unittest.TestCase):
    def setUp(self):
        self.available = [released("v0.9.0"), released("v0.10.0"), released("player-v99.0.0"),
                          released("v1.0.0", draft=True), released("v2.0.0", prerelease=True)]
        self.ancestry = "ahead"
        self.env = patch.dict(os.environ, {
            "GITHUB_REPOSITORY": release_common.REPO,
            "GITHUB_REF": "refs/heads/main",
            "GITHUB_WORKFLOW_REF": release_common.REPO + "/.github/workflows/deploy.yml@refs/heads/main",
            "GITHUB_EVENT_NAME": "push",
            "TAG": "v0.10.0", "AUTOMATIC": "true",
        })
        self.env.start()
        self.addCleanup(self.env.stop)
        external = patch.object(release_common, "command", side_effect=self.github)
        external.start()
        self.addCleanup(external.stop)

    def github(self, *args):
        # Fake only the external gh CLI; all identity and selection code stays real.
        route = next((arg for arg in args if arg.startswith("repos/")), "")
        if route.endswith("releases?per_page=100"):
            return json.dumps([self.available])
        if "/commits/" in route:
            return json.dumps({"sha": SHA})
        if route.endswith(f"compare/{SHA}...main"):
            return json.dumps({"status": self.ancestry})
        self.fail(f"unexpected external operation: {args}")

    def test_latest_go_ignores_player_draft_and_prerelease(self):
        with contextlib.redirect_stdout(io.StringIO()) as output:
            m.main()
        self.assertEqual(dict(line.split("=", 1) for line in output.getvalue().splitlines()),
                         {"sha": SHA, "deploy": "true"})

    def test_old_go_is_inapplicable_for_automatic_deployment(self):
        with contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(m.select("v0.9.0", True), (SHA, False))

    def test_manual_old_go_is_an_explicit_rollback(self):
        os.environ["GITHUB_EVENT_NAME"] = "workflow_dispatch"
        self.assertEqual(m.select("v0.9.0", False), (SHA, True))

    def test_push_cannot_claim_manual_rollback(self):
        with self.assertRaises(ValueError):
            m.select("v0.9.0", False)

    def test_rejects_player_noncanonical_and_unpublished_versions(self):
        for tag in ("player-v99.0.0", "v00.10.0", "v0.10.0-rc.1", "v3.0.0", "v1.0.0", "v2.0.0"):
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                m.select(tag, True)

    def test_rejects_wrong_workflow_repository_ref_or_ancestry(self):
        for field, value in (("GITHUB_REF", "refs/heads/feature"),
                             ("GITHUB_REPOSITORY", "attacker/kite"),
                             ("GITHUB_WORKFLOW_REF", release_common.REPO + "/.github/workflows/ci.yml@refs/heads/main")):
            with self.subTest(field=field), patch.dict(os.environ, {field: value}), self.assertRaises(ValueError):
                m.select("v0.10.0", True)
        self.ancestry = "diverged"
        with self.assertRaises(ValueError):
            m.select("v0.10.0", True)

    def test_malformed_automatic_mode_cannot_bypass_latest_policy(self):
        os.environ["AUTOMATIC"] = ""
        with self.assertRaises(ValueError):
            m.main()


unittest.main()
PY
