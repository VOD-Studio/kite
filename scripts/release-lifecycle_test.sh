#!/usr/bin/env bash
# Isolated API/CLI seams: never contacts a registry or a real release.
set -euo pipefail
RELEASE_SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
export RELEASE_SCRIPT_DIR
python3 - <<'PY'
import importlib.util
import json
import os
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('release', os.environ['RELEASE_SCRIPT_DIR'] + '/release-lifecycle.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

class Lifecycle(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.env = patch.dict(os.environ, {'GITHUB_REPOSITORY': m.REPO, 'GITHUB_REF': 'refs/heads/main', 'TAG': 'v0.5.0', 'GITHUB_OUTPUT': self.tmp.name + '/out', 'CI_POLL_TRIES': '1', 'SOURCE_SHA': 'abc', 'VERIFIED_IMAGE': m.IMAGE + '@sha256:123'})
        self.env.start()
        self.addCleanup(self.env.stop)
        self.release = {'id': 1, 'tag_name': 'v0.5.0', 'draft': True, 'prerelease': False}
        self.commit = {'sha': 'abc', 'parents': [{'sha': 'parent'}]}
        self.pr = {'merged_at': 'now', 'merge_commit_sha': 'abc', 'user': {'login': 'github-actions[bot]', 'type': 'Bot'}, 'base': {'ref': 'main', 'repo': {'full_name': m.REPO}}, 'head': {'ref': 'release-please--branches--main', 'repo': {'full_name': m.REPO}}, 'title': 'chore(main): release 0.5.0'}
        self.run = {'id': 9, 'workflow_id': 5, 'event': 'push', 'head_sha': 'abc', 'head_branch': 'main', 'head_repository': {'full_name': m.REPO}, 'run_number': 1, 'run_attempt': 1, 'status': 'completed', 'conclusion': 'success'}
        self.writes = []
        self.status = 'ahead'
        self.jobs = [{'name': 'CI OK', 'conclusion': 'success'}]
        self.versions = ['v0.5.0']
        self.ap = patch.object(m, 'api', side_effect=self.api)
        self.cp = patch.object(m, 'command', side_effect=self.command)
        self.ap.start(); self.cp.start()
        self.addCleanup(self.ap.stop); self.addCleanup(self.cp.stop)
    def api(self, path, *args):
        if args:
            self.writes.append((path, args)); return {}
        if path.startswith('releases/tags/'): return self.release
        if path.startswith('compare/'): return {'status': self.status}
        if path.endswith('/pulls'): return [self.pr]
        if path.startswith('commits/'): return self.commit
        if path == 'actions/workflows/ci.yml': return {'id': 5}
        if path.startswith('actions/workflows/5/runs?'): return {'workflow_runs': [self.run]}
        raise AssertionError(path)
    def command(self, *args):
        if args[0] == 'docker': self.writes.append(args); return ''
        if '/jobs?' in args[-1]: return json.dumps([{'jobs': self.jobs}])
        if '/releases?' in args[-1]: return json.dumps([[{'tag_name': v, 'draft': False, 'prerelease': False} for v in self.versions]])
        raise AssertionError(args)
    def test_authorized_draft_builds(self):
        m.prepare()
        with open(os.environ['GITHUB_OUTPUT']) as f: self.assertIn('build=true', f.read())
        self.assertEqual(self.writes, [])
    def test_public_never_builds(self):
        self.release['draft'] = False
        self.run['conclusion'] = 'failure'
        m.prepare()
        with open(os.environ['GITHUB_OUTPUT']) as f: self.assertIn('build=false', f.read())
        self.assertEqual(self.writes, [])
    def test_non_main_tooling_refused(self):
        os.environ['GITHUB_REF'] = 'refs/tags/v0.5.0'
        with self.assertRaises(ValueError): m.prepare()
    def test_fork_tooling_refused(self):
        os.environ['GITHUB_REPOSITORY'] = 'attacker/kite'
        with self.assertRaises(ValueError): m.prepare()
    def test_untrusted_source_refused(self):
        self.status = 'diverged'
        with self.assertRaises(ValueError): m.prepare()
    def test_merge_release_commit_refused(self):
        self.commit['parents'].append({'sha': 'second'})
        with self.assertRaises(ValueError): m.prepare()
    def test_same_name_fork_release_pr_refused(self):
        self.pr['head']['repo']['full_name'] = 'attacker/kite'
        with self.assertRaises(ValueError): m.prepare()
    def test_non_bot_release_refused(self):
        self.pr['user']['type'] = 'User'
        with self.assertRaises(ValueError): m.prepare()
    def test_other_producer_refused(self):
        self.run['workflow_id'] = 6
        with self.assertRaises(ValueError): m.prepare()
    def test_wrong_commit_refused(self):
        self.run['head_sha'] = 'other'
        with self.assertRaises(ValueError): m.prepare()
    def test_dispatch_is_not_main_ci(self):
        self.run['event'] = 'workflow_dispatch'
        with self.assertRaises(ValueError): m.prepare()
    def test_failed_gate_refused(self):
        self.jobs[0]['conclusion'] = 'failure'
        with self.assertRaises(ValueError): m.prepare()
    def test_old_public_is_no_write(self):
        self.release['draft'] = False
        self.versions = ['v0.5.0', 'v0.5.1', 'v0.6.0']
        m.publish()
        self.assertEqual(self.writes, [])
    def test_old_draft_is_not_latest(self):
        self.versions = ['v0.5.0', 'v0.5.1', 'v0.6.0']
        m.publish()
        self.assertEqual(len(self.writes), 1)
        self.assertIn('make_latest=false', self.writes[0][1])
    def test_public_replay_repairs_only_floating_tags(self):
        self.release['draft'] = False
        m.publish()
        self.assertEqual(len(self.writes), 2)
        self.assertTrue(all(w[0] == 'docker' for w in self.writes))
    def test_tag_changed_before_publish(self):
        os.environ['SOURCE_SHA'] = 'other'
        with self.assertRaises(ValueError): m.publish()
        self.assertEqual(self.writes, [])
    def test_canonical_tags(self):
        for tag in ['main', 'v01.2.3', 'v1.2.3-rc.1', 'v1.2.3\n', 'v1.2.3;echo bad']:
            with self.subTest(tag=tag), self.assertRaises(ValueError): m.version(tag)

unittest.main()
PY
