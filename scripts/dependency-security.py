#!/usr/bin/env python3
"""Run every selected dependency scanner, preserving no-fix advisories on failure."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
SCOPES = ("go", "player", "rust")
SEVERITIES = ("info", "low", "moderate", "high", "critical")


class ScanError(Exception):
    pass


def require(condition, message):
    if not condition:
        raise ScanError(message)


def nonnegative_int(value):
    return type(value) is int and value >= 0


def run(command, cwd, env=None):
    result = subprocess.run(command, cwd=cwd, env=env, text=True, capture_output=True)
    if result.stdout:
        print(result.stdout, end="" if result.stdout.endswith("\n") else "\n")
    if result.stderr:
        print(result.stderr, file=sys.stderr, end="" if result.stderr.endswith("\n") else "\n")
    return result


def json_report(result):
    require(result.returncode in (0, 1), f"扫描工具失败(exit {result.returncode})")
    try:
        report = json.loads(result.stdout)
    except (ValueError, TypeError) as exc:
        raise ScanError("扫描器未返回完整 JSON 报告") from exc
    require(isinstance(report, dict) and "error" not in report, "扫描报告包含错误或不是对象")
    return report


def npm_scan():
    result = run(
        [os.environ.get("NPM", "npm"), "audit", "--json", "--package-lock-only",
         "--audit-level=low", "--include=prod", "--include=dev", "--include=optional", "--include=peer"],
        ROOT / "packages/kite-player",
    )
    report = json_report(result)
    require(report.get("auditReportVersion") == 2, "npm audit 报告版本不可识别")
    vulnerabilities = report.get("vulnerabilities")
    metadata = report.get("metadata")
    require(isinstance(vulnerabilities, dict) and isinstance(metadata, dict), "npm audit 报告不完整")
    counts = metadata.get("vulnerabilities")
    require(isinstance(counts, dict), "npm audit 缺少漏洞统计")
    require(all(nonnegative_int(counts.get(key)) for key in (*SEVERITIES, "total")), "npm audit 漏洞统计损坏")
    require(counts["total"] == len(vulnerabilities) == sum(counts[key] for key in SEVERITIES), "npm audit 漏洞统计不一致")
    fixable = advisory = 0
    observed = dict.fromkeys(SEVERITIES, 0)
    for name, vulnerability in vulnerabilities.items():
        require(isinstance(vulnerability, dict), "npm audit 漏洞记录损坏")
        require(vulnerability.get("name") == name and vulnerability.get("severity") in SEVERITIES, "npm audit 漏洞身份不完整")
        require(isinstance(vulnerability.get("via"), list) and vulnerability["via"], "npm audit 漏洞来源缺失")
        observed[vulnerability["severity"]] += 1
        fix = vulnerability.get("fixAvailable")
        if fix is False:
            advisory += 1
        elif fix is True:
            fixable += 1
        else:
            require(isinstance(fix, dict) and isinstance(fix.get("name"), str) and bool(fix["name"])
                    and isinstance(fix.get("version"), str) and bool(fix["version"])
                    and type(fix.get("isSemVerMajor")) is bool, "npm audit 修复信息缺失或损坏")
            fixable += 1
    require(all(observed[key] == counts[key] for key in SEVERITIES), "npm audit 严重程度统计不一致")
    # --audit-level=low does not fail for informational-only records.
    expected_failure = any(counts[key] for key in SEVERITIES if key != "info")
    require(result.returncode == int(expected_failure), "npm audit 退出状态与报告不一致")
    return fixable > 0, advisory > 0


def rust_scan():
    result = run([os.environ.get("CARGO", "cargo"), "audit", "--json", "--file", "Cargo.lock"], ROOT / "interop/rust")
    report = json_report(result)
    vulnerabilities = report.get("vulnerabilities")
    require(isinstance(vulnerabilities, dict), "cargo audit 缺少漏洞报告")
    findings = vulnerabilities.get("list")
    require(isinstance(findings, list) and nonnegative_int(vulnerabilities.get("count")), "cargo audit 漏洞统计损坏")
    require(vulnerabilities["count"] == len(findings)
            and type(vulnerabilities.get("found")) is bool
            and vulnerabilities["found"] == bool(findings), "cargo audit 漏洞统计不一致")
    require(result.returncode == int(bool(findings)), "cargo audit 退出状态与报告不一致")
    require(isinstance(report.get("database"), dict) and isinstance(report.get("lockfile"), dict)
            and isinstance(report.get("settings"), dict), "cargo audit 报告不完整")
    require(nonnegative_int(report["database"].get("advisory-count"))
            and nonnegative_int(report["lockfile"].get("dependency-count")), "cargo audit 数据库/锁文件统计损坏")
    fixable = advisory = 0
    for finding in findings:
        require(isinstance(finding, dict) and isinstance(finding.get("advisory"), dict)
                and isinstance(finding["advisory"].get("id"), str)
                and isinstance(finding.get("package"), dict)
                and isinstance(finding["package"].get("name"), str), "cargo audit 漏洞身份不完整")
        versions = finding.get("versions")
        require(isinstance(versions, dict), "cargo audit 修复信息缺失")
        for key in ("patched", "unaffected"):
            require(isinstance(versions.get(key), list)
                    and all(isinstance(value, str) and value for value in versions[key]), "cargo audit 修复信息损坏")
        if versions["patched"]:
            fixable += 1
        else:
            advisory += 1
    warnings = report.get("warnings")
    require(isinstance(warnings, dict), "cargo audit 警告报告缺失")
    for entries in warnings.values():
        require(isinstance(entries, list) and all(isinstance(entry, dict)
                and isinstance(entry.get("package"), dict) for entry in entries), "cargo audit 警告报告损坏")
        # Informational/yanked warnings never disappear from the scheduled issue.
        advisory += len(entries)
    return fixable > 0, advisory > 0


def go_scan(directory):
    # Each existing gate owns a private output file: a later clean module cannot
    # overwrite advisory=true in the caller's GITHUB_OUTPUT.
    with tempfile.TemporaryDirectory(prefix="kite-govulncheck-") as temporary:
        output = Path(temporary) / "output"
        environment = dict(os.environ, GITHUB_OUTPUT=str(output))
        result = run(["bash", str(ROOT / "scripts/govulncheck-gate.sh"), "./..."], directory, environment)
        values = output.read_text().splitlines() if output.exists() else []
        fixable = "fixable=true" in values
        require(result.returncode == 0 or fixable, f"govulncheck 门禁失败(exit {result.returncode})")
        return fixable, "advisory=true" in values


def main(args):
    scopes = list(dict.fromkeys(args or SCOPES))
    unknown = set(scopes) - set(SCOPES)
    if unknown:
        print(f"未知 dependency security scope: {', '.join(sorted(unknown))}", file=sys.stderr)
        return 2
    scans = []
    for scope in scopes:
        if scope == "go":
            scans.extend((("go-root", lambda: go_scan(ROOT)),
                          ("go-interop", lambda: go_scan(ROOT / "interop/go"))))
        else:
            scans.append((scope, npm_scan if scope == "player" else rust_scan))
    # report 模式只降级「存在可修复漏洞」；安装、执行和解析故障仍然失败。
    report_only = os.environ.get("DEPENDENCY_SECURITY_MODE") == "report"
    advisory = False
    failures = []
    for label, scan in scans:
        print(f"::group::Dependency security: {label}", flush=True)
        try:
            fixable, has_advisory = scan()
            advisory |= has_advisory
            if has_advisory:
                print(f"::warning::{label}: 无修复版本漏洞或依赖 advisory，需跟踪")
            if fixable and report_only:
                print(f"::warning::{label}: 存在可修复漏洞，report 模式不阻断，由每日定时扫描跟踪")
            elif fixable:
                failures.append(label)
                print(f"::error::{label}: 存在可修复漏洞或扫描失败，请检查报告")
        except (ScanError, OSError, UnicodeError) as exc:
            failures.append(label)
            print(f"::error::{label}: {exc}")
        finally:
            print("::endgroup::", flush=True)
    if os.environ.get("GITHUB_OUTPUT"):
        with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
            output.write(f"advisory={str(advisory).lower()}\n")
            output.write(f"failed_scans={','.join(failures)}\n")
    return int(bool(failures))


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
