#!/usr/bin/env python3
"""Exercise planner and gate CLIs with real Git histories and a fake GitHub API."""
import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
REPO = "VOD-Studio/kite"


class Boundary(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git("init", "-q")
        self.git("config", "user.email", "ci@example.invalid")
        self.git("config", "user.name", "CI boundary test")
        self.git("config", "commit.gpgsign", "false")
        self.put("README.md", "initial\n")
        self.put(".github/workflows/ci.yml", (HERE.parent / ".github/workflows/ci.yml").read_text())
        self.base = self.commit()
        (self.root / ".git/info/exclude").write_text("event.json\nprs.json\nbin/\n")
        self.event_path = self.root / "event.json"
        self.api_path = self.root / "prs.json"
        self.bin = self.root / "bin"
        self.bin.mkdir()
        fake = self.bin / "gh"
        fake.write_text('#!/usr/bin/env python3\nimport os,sys\nassert sys.argv[1] == "api"\nprint(open(os.environ["FAKE_PRS"]).read())\n')
        fake.chmod(0o755)
        self.env = {
            **os.environ, "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
            "GITHUB_REPOSITORY": REPO, "GITHUB_RUN_ID": "123", "GITHUB_RUN_ATTEMPT": "1",
            "GITHUB_EVENT_PATH": str(self.event_path), "FAKE_PRS": str(self.api_path),
        }
        self.env.pop("GITHUB_OUTPUT", None)

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root, text=True, stderr=subprocess.PIPE).strip()

    def put(self, path, text="changed\n"):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text)

    def commit(self):
        # Keep only tracked test fixture paths in commits, never event/API files.
        self.git("add", "--all")
        self.git("commit", "-qm", "test: fixture")
        return self.git("rev-parse", "HEAD")

    def pr(self, paths):
        for path in paths:
            self.put(path)
        head = self.commit()
        pr = {
            "number": 42, "state": "open", "title": "feat: change player",
            "user": {"login": "person", "type": "User"},
            "base": {"ref": "main", "sha": self.base, "repo": {"full_name": REPO}},
            "head": {"ref": "feature", "sha": head, "repo": {"full_name": REPO}},
        }
        self.event = {"repository": {"full_name": REPO}, "pull_request": pr, "action": "synchronize"}
        self.env.update(GITHUB_EVENT_NAME="pull_request", GITHUB_REF="refs/pull/42/merge", GITHUB_SHA=head)
        self.save_event()
        return pr

    def save_event(self):
        self.event_path.write_text(json.dumps(self.event))

    def run_cli(self, mode, needs=None):
        args = [sys.executable, str(HERE / "ci-plan.py"), "plan"] if mode == "plan" else ["bash", str(HERE / "ci-results.sh")]
        return subprocess.run(args, cwd=self.root, env=self.env, input=None if needs is None else json.dumps(needs), text=True, capture_output=True)

    def plan(self):
        result = self.run_cli("plan")
        self.assertEqual(result.returncode, 0, result.stderr)
        return json.loads(result.stdout)

    def selected(self, value):
        return {name for name, enabled in value["tasks"].items() if enabled}

    def release_dispatch(self, component="player"):
        pr = self.pr(["packages/kite-player/package.json"])
        pr["user"] = {"login": "github-actions[bot]", "type": "Bot"}
        pr["title"] = f"chore({component}): release 1.2.3"
        pr["head"]["ref"] = "release-please--branches--main--components--" + component
        self.env.update(GITHUB_EVENT_NAME="workflow_dispatch", GITHUB_REF="refs/heads/" + pr["head"]["ref"])
        self.event = {"repository": {"full_name": REPO}}
        self.save_event()
        self.api_path.write_text(json.dumps([pr]))
        return pr

    def green(self):
        value = self.plan()
        needs = {name: {"result": "success" if required else "skipped"} for name, required in value["tasks"].items()}
        needs["plan"] = {"result": "success", "outputs": {"plan": json.dumps(value)}}
        return needs

    def admits(self, needs, expected):
        result = self.run_cli("check", needs)
        self.assertEqual(result.returncode == 0, expected, result.stdout + result.stderr)


class PlanTests(Boundary):
    def test_player_source_does_not_start_unrelated_go_jobs(self):
        self.pr(["packages/kite-player/src/element/kite-player.ts"])
        self.assertEqual(self.selected(self.plan()), {"player", "metadata"})

    def test_player_dependency_scope_is_not_go_or_rust(self):
        self.pr(["packages/kite-player/package-lock.json"])
        value = self.plan()
        self.assertEqual(self.selected(value), {"player", "security", "metadata"})
        self.assertEqual(value["securityModules"], ["player"])

    def test_complete_pr_diff_survives_last_commit_and_title_edit(self):
        self.put("internal/server/server.go")
        self.commit()
        self.pr(["packages/kite-player/src/element/player.css"])
        before = self.plan()
        self.assertTrue(before["tasks"]["go-test"])
        self.event["action"] = "edited"
        self.event["pull_request"]["title"] = "fix: edited title"
        self.save_event()
        self.assertEqual(before, self.plan())

    def test_main_is_full_even_for_plain_documentation(self):
        self.pr(["README.md"])
        self.env.update(GITHUB_EVENT_NAME="push", GITHUB_REF="refs/heads/main")
        self.event = {"repository": {"full_name": REPO}, "before": self.base, "after": self.env["GITHUB_SHA"], "deleted": False}
        self.save_event()
        value = self.plan()
        self.assertTrue(all(enabled for name, enabled in value["tasks"].items() if name not in ("security", "metadata")))
        self.assertFalse(value["tasks"]["metadata"])
        self.assertFalse(value["tasks"]["security"])

    def test_control_plane_selects_everything_and_all_dependency_modules(self):
        self.pr(["scripts/helper.sh"])
        value = self.plan()
        self.assertTrue(all(value["tasks"].values()))
        self.assertEqual(value["securityModules"], ["go", "player", "rust"])

    def test_public_dto_selects_both_consumers(self):
        self.pr(["internal/public/dto.go"])
        selected = self.selected(self.plan())
        self.assertTrue({"go-test", "player"} <= selected)

    def test_public_player_adapter_selects_go_contract_guard(self):
        self.pr(["packages/kite-player/src/core/source.ts"])
        self.assertTrue({"go-test", "player"} <= self.selected(self.plan()))

    def test_player_protocol_documentation_checks_both_consumers(self):
        self.pr(["docs/player.md"])
        self.assertTrue({"go-test", "player"} <= self.selected(self.plan()))

    def test_new_job_outside_the_registry_cannot_escape_the_gate(self):
        self.pr(["README.md"])
        workflow = self.root / ".github/workflows/ci.yml"
        with workflow.open("a") as output:
            output.write("\n  unregistered:\n    runs-on: ubuntu-24.04\n    steps:\n      - run: exit 1\n")
        self.assertNotEqual(self.run_cli("plan").returncode, 0)

    def test_protocol_selects_go_rust_player(self):
        self.pr(["proto/netease/player.proto"])
        self.assertTrue({"proto", "go-interop", "rust-interop", "go-test", "player"} <= self.selected(self.plan()))

    def test_plain_docs_only_require_metadata(self):
        self.pr(["docs/adr/decision.md"])
        self.assertEqual(self.selected(self.plan()), {"metadata"})

    def test_shared_executable_docs_check_both_language_consumers(self):
        self.pr(["docs/integration.md"])
        self.assertTrue({"go-test", "go-interop", "rust-interop"} <= self.selected(self.plan()))

    def test_rust_executable_docs_are_not_plain_docs(self):
        self.pr(["docs/server-security.md"])
        self.assertEqual(self.selected(self.plan()), {"metadata", "go-test", "rust-interop"})

    def test_cli_generated_docs_and_llms_links_keep_go_guard(self):
        self.pr(["docs/cmd/kite.md", "llms.txt"])
        self.assertEqual(self.selected(self.plan()), {"metadata", "go-test"})

    def test_unknown_path_is_conservative(self):
        self.pr(["new-subsystem/unknown.data"])
        value = self.plan()
        self.assertTrue(all(enabled for name, enabled in value["tasks"].items() if name != "security"))

    def test_cross_scope_rename_preserves_deleted_scope(self):
        self.put("internal/public/old.go", "old content")
        self.base = self.commit()
        self.git("mv", "-f", "internal/public/old.go", "README.md")
        head = self.commit()
        self.pr(["docs/ordinary.md"])
        value = self.plan()
        self.assertIn("internal/public/old.go", value["context"]["paths"])
        self.assertTrue(value["tasks"]["player"])
        self.assertNotEqual(head, self.base)

    def test_trusted_release_dispatch_requires_metadata(self):
        self.release_dispatch()
        self.assertTrue(self.plan()["tasks"]["metadata"])

    def test_trusted_kite_release_dispatch_requires_metadata(self):
        self.release_dispatch("kite")
        self.assertTrue(self.plan()["tasks"]["metadata"])

    def test_release_dispatch_rejects_legacy_open_titles(self):
        pr = self.release_dispatch()
        for component, title in (("kite", "chore(main): release 1.2.3"),
                                 ("player", "chore(main): release player 1.2.3")):
            with self.subTest(component=component):
                pr["head"]["ref"] = "release-please--branches--main--components--" + component
                self.env["GITHUB_REF"] = "refs/heads/" + pr["head"]["ref"]
                pr["title"] = title
                self.api_path.write_text(json.dumps([pr]))
                self.assertNotEqual(self.run_cli("plan").returncode, 0)

    def test_release_dispatch_rejects_identity_title_sha_and_duplicate_pr(self):
        pr = self.release_dispatch()
        variants = []
        for field, value in (("title", "chore(kite): release 1.2.3"), ("user", {"login": "attacker", "type": "User"})):
            changed = copy.deepcopy(pr)
            changed[field] = value
            variants.append([changed])
        changed = copy.deepcopy(pr)
        changed["head"]["sha"] = "a" * 40
        variants.extend(([changed], [pr, pr], []))
        changed = copy.deepcopy(pr)
        changed["head"]["repo"]["full_name"] = "attacker/kite"
        variants.append([changed])
        changed = copy.deepcopy(pr)
        changed["state"] = "closed"
        variants.append([changed])
        for prs in variants:
            with self.subTest(prs=prs):
                self.api_path.write_text(json.dumps(prs))
                self.assertNotEqual(self.run_cli("plan").returncode, 0)

    def test_wrong_push_ref_is_rejected(self):
        self.pr(["README.md"])
        self.env.update(GITHUB_EVENT_NAME="push", GITHUB_REF="refs/heads/feature")
        self.assertNotEqual(self.run_cli("plan").returncode, 0)

    def test_missing_git_base_cannot_become_empty_plan(self):
        self.pr(["README.md"])
        self.event["pull_request"]["base"]["sha"] = "a" * 40
        self.save_event()
        self.assertNotEqual(self.run_cli("plan").returncode, 0)

    def test_checkout_must_match_run_source(self):
        self.pr(["README.md"])
        self.env["GITHUB_SHA"] = "a" * 40
        self.assertNotEqual(self.run_cli("plan").returncode, 0)

    def test_arbitrary_branch_dispatch_is_not_a_metadata_bypass(self):
        self.pr(["README.md"])
        self.env.update(GITHUB_EVENT_NAME="workflow_dispatch", GITHUB_REF="refs/heads/feature")
        self.assertNotEqual(self.run_cli("plan").returncode, 0)


class GateTests(Boundary):
    def setUp(self):
        super().setUp()
        self.pr(["packages/kite-player/src/element/kite-player.ts"])

    def test_explicitly_inapplicable_jobs_can_skip(self):
        self.admits(self.green(), True)

    def test_selected_failure_cancel_skip_and_unknown_result_block(self):
        for result in ("failure", "cancelled", "skipped", "neutral", None):
            with self.subTest(result=result):
                needs = self.green()
                needs["player"]["result"] = result
                self.admits(needs, False)

    def test_optional_failure_is_not_hidden(self):
        needs = self.green()
        needs["go-test"]["result"] = "failure"
        self.admits(needs, False)

    def test_metadata_cannot_skip(self):
        needs = self.green()
        needs["metadata"]["result"] = "skipped"
        self.admits(needs, False)

    def test_missing_unknown_and_failed_plan_block(self):
        original = self.green()
        for key in original:
            needs = copy.deepcopy(original)
            del needs[key]
            self.admits(needs, False)
        needs = copy.deepcopy(original)
        needs["unclassified"] = {"result": "success"}
        self.admits(needs, False)
        for result in ("failure", "cancelled", "skipped"):
            needs = copy.deepcopy(original)
            needs["plan"]["result"] = result
            self.admits(needs, False)

    def test_forged_plan_cannot_authorize_skip(self):
        needs = self.green()
        value = json.loads(needs["plan"]["outputs"]["plan"])
        value["tasks"]["player"] = False
        needs["plan"]["outputs"]["plan"] = json.dumps(value)
        needs["player"]["result"] = "skipped"
        self.admits(needs, False)

    def test_malformed_unknown_and_missing_plan_fields_block(self):
        needs = self.green()
        valid = json.loads(needs["plan"]["outputs"]["plan"])
        variants = ["", "{}", "null", "{bad", '{"schemaVersion":1,"schemaVersion":1}']
        for field in valid:
            value = copy.deepcopy(valid)
            del value[field]
            variants.append(json.dumps(value))
        value = copy.deepcopy(valid)
        value["tasks"]["extra"] = False
        variants.append(json.dumps(value))
        value = copy.deepcopy(valid)
        value["tasks"]["player"] = 1
        variants.append(json.dumps(value))
        for encoded in variants:
            with self.subTest(encoded=encoded):
                needs["plan"]["outputs"]["plan"] = encoded
                self.admits(needs, False)

    def test_different_run_sha_and_attempt_are_rejected(self):
        original = self.green()
        for field, value in (("runId", "999"), ("runAttempt", 2), ("sourceSha", "a" * 40)):
            needs = copy.deepcopy(original)
            plan = json.loads(needs["plan"]["outputs"]["plan"])
            plan["context"][field] = value
            needs["plan"]["outputs"]["plan"] = json.dumps(plan)
            self.admits(needs, False)

    def test_title_edit_does_not_wash_failed_code_check(self):
        needs = self.green()
        needs["player"]["result"] = "failure"
        self.event["action"] = "edited"
        self.event["pull_request"]["title"] = "fix: now conventional"
        self.save_event()
        self.admits(needs, False)

    def test_release_dispatch_metadata_skip_fails(self):
        self.release_dispatch()
        needs = self.green()
        self.admits(needs, True)
        needs["metadata"]["result"] = "skipped"
        self.admits(needs, False)

    def test_main_requires_all_deterministic_jobs(self):
        self.env.update(GITHUB_EVENT_NAME="push", GITHUB_REF="refs/heads/main")
        self.event = {"repository": {"full_name": REPO}, "before": self.base, "after": self.env["GITHUB_SHA"], "deleted": False}
        self.save_event()
        needs = self.green()
        self.admits(needs, True)
        self.assertEqual(needs["metadata"]["result"], "skipped")
        needs["go-test"]["result"] = "skipped"
        self.admits(needs, False)


if __name__ == "__main__":
    unittest.main()
