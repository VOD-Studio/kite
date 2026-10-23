#!/usr/bin/env bash
# Real disposable git repositories; never touches the working repository's refs.
set -euo pipefail
RELEASE_SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
export RELEASE_SCRIPT_DIR
python3 - <<'PY'
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = str(Path(os.environ['RELEASE_SCRIPT_DIR']) / 'go-release-tags.py')

class Tags(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(); self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.env = dict(os.environ, GIT_AUTHOR_NAME='Fixture', GIT_AUTHOR_EMAIL='fixture@example.invalid',
                        GIT_COMMITTER_NAME='Fixture', GIT_COMMITTER_EMAIL='fixture@example.invalid')
        self.git('init', '-q')
        self.commit()
        self.first = self.git('rev-parse', 'HEAD')

    def git(self, *args):
        return subprocess.check_output(['git', *args], cwd=self.root, env=self.env, text=True).strip()

    def commit(self):
        self.git('commit', '--quiet', '--allow-empty', '-m', 'feat: fixture')

    def select(self, tag=None, success=True):
        args = ['python3', SCRIPT] + ([] if tag is None else [tag])
        before = self.git('show-ref', '--head')
        result = subprocess.run(args, cwd=self.root, env=self.env, text=True, capture_output=True)
        self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)
        self.assertEqual(before, self.git('show-ref', '--head'), 'selection must never mutate refs')
        if success:
            return dict(line.split('=', 1) for line in result.stdout.splitlines())
        self.assertEqual(result.stdout, '')

    def test_no_go_history_uses_snapshot_base_and_real_root(self):
        self.git('tag', 'player-v99.0.0')
        self.git('tag', 'v01.2.3')
        self.git('tag', 'v9.0.0-rc.1')
        self.assertEqual(self.select(), {'GORELEASER_CURRENT_TAG': 'v0.0.0', 'GORELEASER_PREVIOUS_TAG': self.first})

    def test_player_head_and_same_commit_tags_do_not_pollute_snapshot(self):
        self.git('tag', 'v0.6.0')
        self.commit(); self.git('tag', 'v0.7.0'); self.git('tag', 'player-v99.0.0')
        self.commit(); self.git('tag', 'player-v100.0.0')
        self.assertEqual(self.select(), {'GORELEASER_CURRENT_TAG': 'v0.7.0', 'GORELEASER_PREVIOUS_TAG': 'v0.6.0'})

    def test_formal_first_release_never_selects_player_as_previous(self):
        self.git('tag', 'player-v99.0.0'); self.commit(); self.git('tag', 'v0.1.0')
        self.assertEqual(self.select('v0.1.0'), {'GORELEASER_CURRENT_TAG': 'v0.1.0', 'GORELEASER_PREVIOUS_TAG': self.first})

    def test_formal_source_exact_and_player_never_go(self):
        self.git('tag', 'v0.1.0'); self.git('tag', 'player-v1.0.0')
        self.select('player-v1.0.0', success=False)
        self.select('v01.0.0', success=False)
        self.commit()
        self.select('v0.1.0', success=False)

    def test_unreachable_go_tag_is_not_snapshot_source(self):
        self.git('tag', 'v0.1.0'); self.git('checkout', '-q', '-b', 'side')
        self.commit(); self.git('tag', 'v99.0.0')
        self.git('checkout', '-q', '--detach', self.first)
        self.assertEqual(self.select()['GORELEASER_CURRENT_TAG'], 'v0.1.0')

    def test_semantic_sort_and_annotated_tags(self):
        self.git('tag', '-a', 'v0.9.0', '-m', 'old')
        self.commit(); self.git('tag', '-a', 'v0.10.0', '-m', 'new')
        self.git('tag', 'player-v999.0.0')
        self.assertEqual(self.select('v0.10.0'), {'GORELEASER_CURRENT_TAG': 'v0.10.0', 'GORELEASER_PREVIOUS_TAG': 'v0.9.0'})

unittest.main()
PY
