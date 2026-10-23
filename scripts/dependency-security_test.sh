#!/usr/bin/env bash
# Exercise the real entrypoint with external scanner CLI fixtures; no network.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
python3 - "$here" <<'PY'
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

SCRIPTS = Path(sys.argv.pop()).resolve()
ROOT = SCRIPTS.parent


def npm_report(fixes=()):
    vulnerabilities = {}
    for index, fix in enumerate(fixes):
        name = f"dependency-{index}"
        vulnerabilities[name] = {"name": name, "severity": "high", "via": [{"source": index, "name": name}], "fixAvailable": fix}
    return {"auditReportVersion": 2, "vulnerabilities": vulnerabilities,
            "metadata": {"vulnerabilities": {"info": 0, "low": 0, "moderate": 0, "high": len(fixes), "critical": 0, "total": len(fixes)}}}


def rust_report(patches=()):
    findings = [{"advisory": {"id": f"RUSTSEC-2099-{index:04d}"}, "package": {"name": "crate"},
                 "versions": {"patched": patched, "unaffected": []}} for index, patched in enumerate(patches)]
    return {"database": {"advisory-count": 100}, "lockfile": {"dependency-count": 10}, "settings": {},
            "vulnerabilities": {"found": bool(findings), "count": len(findings), "list": findings}, "warnings": {}}


def fixture(report, code=0):
    return {"stdout": report if isinstance(report, str) else json.dumps(report), "code": code}


class SecurityTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.calls = self.path / "calls"
        self.output = self.path / "output"
        scanner = self.path / "scanner"
        scanner.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
name = pathlib.Path(sys.argv[0]).name
cwd = pathlib.Path.cwd()
key = ("go-interop" if cwd.name == "go" else "go-root") if name == "govulncheck" else name
with open(os.environ["CALLS"], "a") as calls:
    calls.write(json.dumps({"key": key, "cwd": str(cwd), "args": sys.argv[1:]}) + "\\n")
case = json.loads(os.environ["SCANNERS"])[key]
print(case.get("stdout", ""), end="")
print(case.get("stderr", ""), end="", file=sys.stderr)
sys.exit(case["code"])
''')
        scanner.chmod(0o755)
        for name in ("govulncheck", "npm", "cargo"):
            (self.path / name).symlink_to(scanner)
        self.scanners = {"go-root": fixture("No vulnerabilities found."), "go-interop": fixture("No vulnerabilities found."),
                         "npm": fixture(npm_report()), "cargo": fixture(rust_report())}

    def invoke(self, scopes=(), expected=0, advisory=False):
        env = dict(os.environ, GOVULNCHECK=str(self.path / "govulncheck"), NPM=str(self.path / "npm"),
                   CARGO=str(self.path / "cargo"), CALLS=str(self.calls), SCANNERS=json.dumps(self.scanners),
                   GITHUB_OUTPUT=str(self.output))
        result = subprocess.run(["bash", str(SCRIPTS / "dependency-security.sh"), *scopes],
                                cwd=self.path, env=env, text=True, capture_output=True)
        self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
        if expected != 2:
            lines = self.output.read_text().splitlines()
            self.assertEqual([line for line in lines if line.startswith("advisory=")],
                             [f"advisory={str(advisory).lower()}"], result.stdout)
            if advisory:
                self.assertIn("::warning::", result.stdout)
        return [json.loads(line) for line in self.calls.read_text().splitlines()] if self.calls.exists() else []

    def test_default_runs_all_modules_in_correct_directories(self):
        calls = self.invoke()
        self.assertEqual([call["key"] for call in calls], ["go-root", "go-interop", "npm", "cargo"])
        self.assertEqual([call["cwd"] for call in calls], [str(ROOT), str(ROOT / "interop/go"),
                         str(ROOT / "packages/kite-player"), str(ROOT / "interop/rust")])
        self.assertEqual(calls[0]["args"], ["./..."])
        self.assertIn("--package-lock-only", calls[2]["args"])
        self.assertIn("--include=dev", calls[2]["args"])
        self.assertEqual(calls[3]["args"], ["audit", "--json", "--file", "Cargo.lock"])

    def test_scopes_deduplicate_and_unknown_rejected_before_scanning(self):
        self.assertEqual(self.invoke(("go", "typo"), expected=2), [])
        calls = self.invoke(("player", "player"))
        self.assertEqual([call["key"] for call in calls], ["npm"])

    def test_go_no_fix_not_overwritten_by_later_clean_scanners(self):
        self.scanners["go-root"] = fixture("Vulnerability #1: GO-2099-0001\n    Fixed in: N/A", 3)
        self.invoke(advisory=True)

    def test_go_interop_fixable_does_not_stop_other_scans(self):
        self.scanners["go-interop"] = fixture("Vulnerability #1: GO-2099-0001\n    Fixed in: example.org/lib@v2.0.0", 3)
        calls = self.invoke(expected=1)
        self.assertEqual(len(calls), 4)
        self.assertIn("failed_scans=go-interop", self.output.read_text())

    def test_go_failure_does_not_hide_later_advisory(self):
        self.scanners["go-root"] = fixture("network unavailable", 1)
        self.scanners["npm"] = fixture(npm_report((False,)), 1)
        self.assertEqual(len(self.invoke(expected=1, advisory=True)), 4)

    def test_npm_fixable_true(self):
        self.scanners["npm"] = fixture(npm_report((True,)), 1)
        self.invoke(("player",), expected=1)

    def test_npm_fixable_version_and_mixed_advisory(self):
        self.scanners["npm"] = fixture(npm_report((False, {"name": "replacement", "version": "2.0.0", "isSemVerMajor": True})), 1)
        self.invoke(("player",), expected=1, advisory=True)

    def test_npm_no_fix(self):
        self.scanners["npm"] = fixture(npm_report((False,)), 1)
        self.invoke(("player",), advisory=True)

    def test_rust_fixable(self):
        self.scanners["cargo"] = fixture(rust_report(([">=2.0.0"],)), 1)
        self.invoke(("rust",), expected=1)

    def test_rust_no_fix(self):
        self.scanners["cargo"] = fixture(rust_report(([],)), 1)
        self.invoke(("rust",), advisory=True)

    def test_rust_mixed_findings(self):
        self.scanners["cargo"] = fixture(rust_report(([], [">=2.0.0"])), 1)
        self.invoke(("rust",), expected=1, advisory=True)

    def test_rust_informational_warning_retained(self):
        report = rust_report()
        report["warnings"] = {"unmaintained": [{"package": {"name": "abandoned"}}]}
        self.scanners["cargo"] = fixture(report)
        self.invoke(("rust",), advisory=True)

    def test_scanner_network_empty_and_malformed_reports_fail(self):
        for scope, command in (("player", "npm"), ("rust", "cargo")):
            for report, code in (("", 0), ("", 1), ("{broken", 1), ({}, 0),
                                 ({"error": {"code": "ENETUNREACH"}}, 1), ("network timeout", 2),
                                 ([], 0)):
                with self.subTest(scope=scope, report=report, code=code):
                    self.output.unlink(missing_ok=True)
                    self.scanners[command] = fixture(report, code)
                    self.invoke((scope,), expected=1)

    def test_npm_missing_fix_or_inconsistent_report_is_failure(self):
        report = npm_report((False,))
        del report["vulnerabilities"]["dependency-0"]["fixAvailable"]
        inconsistent = npm_report((False,))
        inconsistent["metadata"]["vulnerabilities"]["total"] = 0
        for invalid, code in ((report, 1), (inconsistent, 1), (npm_report(), 1), (npm_report((False,)), 0)):
            with self.subTest(report=invalid, code=code):
                self.output.unlink(missing_ok=True)
                self.scanners["npm"] = fixture(invalid, code)
                self.invoke(("player",), expected=1)

    def test_rust_missing_fix_or_inconsistent_report_is_failure(self):
        report = rust_report(([],))
        del report["vulnerabilities"]["list"][0]["versions"]
        inconsistent = rust_report(([],))
        inconsistent["vulnerabilities"]["count"] = 0
        for invalid, code in ((report, 1), (inconsistent, 1), (rust_report(), 1), (rust_report(([],)), 0),
                              (rust_report(([],)), 2)):
            with self.subTest(report=invalid, code=code):
                self.output.unlink(missing_ok=True)
                self.scanners["cargo"] = fixture(invalid, code)
                self.invoke(("rust",), expected=1)

    def test_multiple_failures_and_advisories_are_aggregated_once(self):
        self.scanners["go-root"] = fixture("Vulnerability #1: GO-2099-0001\n    Fixed in: N/A", 3)
        self.scanners["go-interop"] = fixture("network unavailable", 1)
        self.scanners["npm"] = fixture(npm_report((True, False)), 1)
        self.scanners["cargo"] = fixture(rust_report(([">=2.0.0"],)), 1)
        self.assertEqual(len(self.invoke(expected=1, advisory=True)), 4)
        self.assertIn("failed_scans=go-interop,player,rust", self.output.read_text())

    def test_missing_scanner_still_runs_remaining_scopes(self):
        (self.path / "npm").unlink()
        calls = self.invoke(expected=1)
        self.assertEqual([call["key"] for call in calls], ["go-root", "go-interop", "cargo"])
        self.assertIn("failed_scans=player", self.output.read_text())


unittest.main()
PY
