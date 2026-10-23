#!/usr/bin/env bash
# Isolated API/CLI seams: never contacts a registry or a real release.
set -euo pipefail
RELEASE_SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
export RELEASE_SCRIPT_DIR
export PYTHONPATH="$RELEASE_SCRIPT_DIR${PYTHONPATH:+:$PYTHONPATH}"
python3 - <<'PY'
import importlib.util
import os
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('release', os.environ['RELEASE_SCRIPT_DIR'] + '/release-lifecycle.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
SHA = 'a' * 40

class Lifecycle(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(); self.addCleanup(self.tmp.cleanup)
        p = patch.dict(os.environ, {'GITHUB_REPOSITORY': m.REPO, 'GITHUB_REF': 'refs/heads/main', 'GITHUB_WORKFLOW_REF': m.REPO + '/.github/workflows/release.yml@refs/heads/main', 'TAG': 'v0.7.0', 'GITHUB_OUTPUT': self.tmp.name + '/out', 'SOURCE_SHA': SHA, 'VERIFIED_IMAGE': m.IMAGE + '@sha256:' + '1' * 64})
        p.start(); self.addCleanup(p.stop)
        self.release = {'id': 1, 'tag_name': 'v0.7.0', 'draft': True, 'prerelease': False}
        self.commit = {'sha': SHA, 'parents': [{'sha': 'b' * 40}]}
        self.versions = []
        self.writes = []
        self.auth = []
        self.ci = []
        mocks = [patch.object(m.common, 'release_for', side_effect=lambda tag: self.release),
                 patch.object(m.common, 'resolve_source', side_effect=lambda tag: self.commit),
                 patch.object(m.common, 'require_release_pr', side_effect=lambda *args: self.auth.append(args)),
                 patch.object(m.common, 'wait_main_ci', side_effect=lambda sha: self.ci.append(sha)),
                 patch.object(m.common, 'releases', side_effect=self.listing),
                 patch.object(m.common, 'api', side_effect=self.api),
                 patch.object(m.common, 'command', side_effect=lambda *args: self.writes.append(args))]
        for mock in mocks:
            mock.start(); self.addCleanup(mock.stop)

    def listing(self):
        return [self.release] + [{'tag_name': tag, 'draft': False, 'prerelease': False} for tag in self.versions]

    def api(self, path, *args):
        self.writes.append((path, args))
        self.release['draft'] = False
        return self.release

    def test_authorized_draft_builds_exact_source(self):
        m.prepare()
        with open(os.environ['GITHUB_OUTPUT']) as stream: out = stream.read()
        self.assertIn('build=true', out); self.assertIn('sha=' + SHA, out)
        self.assertEqual(self.auth, [(self.commit, 'kite', '0.7.0')])
        self.assertEqual(self.ci, [SHA]); self.assertEqual(self.writes, [])

    def test_public_history_never_builds_or_overwrites_assets(self):
        self.release['draft'] = False; self.versions = ['v0.7.1', 'v0.8.0']
        m.prepare(); m.publish()
        self.assertEqual(self.writes, []); self.assertEqual(self.auth, []); self.assertEqual(self.ci, [])
        with open(os.environ['GITHUB_OUTPUT']) as stream: self.assertNotIn('build=true', stream.read())

    def test_public_replay_repairs_only_applicable_floating_tags(self):
        self.release['draft'] = False
        m.prepare(); m.publish()
        self.assertEqual(len(self.writes), 2)
        self.assertTrue(all(call[:4] == ('docker', 'buildx', 'imagetools', 'create') for call in self.writes))
        self.assertEqual([call[-2] for call in self.writes], [m.IMAGE + ':latest', m.IMAGE + ':v0.7'])
        self.assertTrue(all(call[-1] == os.environ['VERIFIED_IMAGE'] for call in self.writes))
        self.assertEqual(self.auth, []); self.assertEqual(self.ci, [])

    def test_player_never_enters_go_lifecycle(self):
        os.environ['TAG'] = 'player-v0.7.0'
        for action in (m.prepare, m.publish):
            with self.assertRaises(ValueError): action()
        self.assertEqual(self.writes, [])

    def test_draft_publish_promotes_only_forward(self):
        self.versions = ['v0.6.0', 'player-v99.0.0']
        m.publish()
        self.assertEqual(len(self.writes), 3)
        self.assertIn('make_latest=true', self.writes[0][1])
        self.assertEqual(self.writes[1][-2], m.IMAGE + ':latest')
        self.assertEqual(self.writes[2][-2], m.IMAGE + ':v0.7')

    def test_old_draft_does_not_move_floating_tags_backwards(self):
        self.versions = ['v0.7.1', 'v0.8.0']
        m.publish()
        self.assertEqual(len(self.writes), 1)
        self.assertIn('make_latest=false', self.writes[0][1])

    def test_older_minor_advances_only_its_minor(self):
        self.versions = ['v0.6.0', 'v0.8.0']
        m.publish()
        self.assertEqual(len(self.writes), 2)
        self.assertIn('make_latest=false', self.writes[0][1])
        self.assertEqual(self.writes[1][-2], m.IMAGE + ':v0.7')

    def test_tag_changed_refused_before_any_write(self):
        os.environ['SOURCE_SHA'] = 'c' * 40
        with self.assertRaises(ValueError): m.publish()
        self.assertEqual(self.writes, [])

    def test_failed_auth_or_ci_never_builds_or_publishes(self):
        for method in ('require_release_pr', 'wait_main_ci'):
            with patch.object(m.common, method, side_effect=ValueError('refused')):
                for action in (m.prepare, m.publish):
                    with self.assertRaises(ValueError): action()
        self.assertEqual(self.writes, [])

unittest.main()
PY
