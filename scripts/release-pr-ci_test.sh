#!/usr/bin/env bash
set -euo pipefail
RELEASE_SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
export RELEASE_SCRIPT_DIR
python3 - <<'PY'
import copy
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

REPO = 'VOD-Studio/kite'
SHA = 'a' * 40
SHIM = r'''#!/usr/bin/env python3
import json, os, sys
args = sys.argv[1:]
with open(os.environ['STATE']) as stream: state = json.load(stream)
with open(os.environ['CALLS'], 'a') as stream: stream.write(json.dumps(args) + '\n')
if args[0] == 'workflow': sys.exit(0)
path = next(arg for arg in args if arg.startswith('repos/'))
if '/pulls?state=' in path:
    component = path.split('--components--')[1].split('&')[0]
    result = [[pr for pr in state['prs'] if pr['head']['ref'].endswith('--' + component)]]
elif path.endswith('/actions/workflows/ci.yml'):
    result = {'id': 5, 'path': '.github/workflows/ci.yml', 'state': 'active'}
elif '/actions/workflows/5/runs?' in path: result = [{'workflow_runs': state['runs']}]
elif '/pulls/' in path: result = state.get('current') or next(pr for pr in state['prs'] if pr['number'] == int(path.rsplit('/', 1)[1]))
elif path.endswith('/approve'): sys.exit(0)
else: sys.exit('unexpected API ' + path)
print(json.dumps(result))
'''

class Dispatch(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(); self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        gh = self.root / 'gh'; gh.write_text(SHIM); gh.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.root) + ':' + os.environ['PATH'], GH=str(gh), GH_REPO=REPO,
                        GITHUB_REPOSITORY=REPO, GITHUB_REF='refs/heads/main',
                        GITHUB_WORKFLOW_REF=REPO + '/.github/workflows/release-please.yml@refs/heads/main',
                        POLL_TRIES='1', POLL_SLEEP='0', STATE=str(self.root / 'state'), CALLS=str(self.root / 'calls'))
        self.pr = {'number': 7, 'state': 'open', 'title': 'chore(kite): release 0.7.0',
                   'user': {'login': 'github-actions[bot]', 'type': 'Bot'},
                   'base': {'ref': 'main', 'repo': {'full_name': REPO}},
                   'head': {'sha': SHA, 'ref': 'release-please--branches--main--components--kite', 'repo': {'full_name': REPO}}}
        self.run = {'id': 9, 'workflow_id': 5, 'path': '.github/workflows/ci.yml', 'head_sha': SHA,
                    'head_branch': self.pr['head']['ref'], 'head_repository': {'full_name': REPO},
                    'event': 'pull_request', 'pull_requests': [{'number': 7}], 'conclusion': 'action_required'}
        self.state = {'prs': [self.pr], 'runs': [self.run]}

    def invoke(self, expected=0):
        Path(self.env['STATE']).write_text(json.dumps(self.state))
        Path(self.env['CALLS']).write_text('')
        result = subprocess.run(['bash', os.environ['RELEASE_SCRIPT_DIR'] + '/release-pr-ci.sh'], env=self.env, text=True, capture_output=True)
        self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
        calls = [json.loads(line) for line in Path(self.env['CALLS']).read_text().splitlines()]
        return [call for call in calls if call[0] == 'workflow' or any('/approve' in arg for arg in call)]

    def test_each_component_approves_and_dispatches(self):
        for component in ('kite', 'player'):
            self.pr['head']['ref'] = 'release-please--branches--main--components--' + component
            self.pr['title'] = f'chore({component}): release 0.7.0'
            self.run['head_branch'] = self.pr['head']['ref']
            self.state['runs'] = [self.run]
            calls = self.invoke(); self.assertEqual(len(calls), 1); self.assertIn('/approve', ' '.join(calls[0]))
            self.state['runs'] = []
            self.assertEqual(self.invoke(), [['workflow', 'run', 'ci.yml', '--ref', self.pr['head']['ref']]])

    def test_both_releases_dispatched_independently(self):
        player = copy.deepcopy(self.pr)
        player.update(number=8, title='chore(player): release 0.2.0')
        player['head']['ref'] = 'release-please--branches--main--components--player'
        self.state['prs'].append(player); self.state['runs'] = []
        self.assertEqual([call[-1] for call in self.invoke()], [self.pr['head']['ref'], player['head']['ref']])

    def test_exact_completed_or_dispatch_producer_is_idempotent(self):
        self.run['conclusion'] = 'success'
        self.assertEqual(self.invoke(), [])
        self.run.update(event='workflow_dispatch', pull_requests=[])
        self.assertEqual(self.invoke(), [])

    def test_other_sha_or_producer_does_not_authorize_approval(self):
        original = copy.deepcopy(self.run)
        for key, value in [('workflow_id', 99), ('head_sha', 'b' * 40), ('path', '.github/workflows/ai.yml'), ('head_repository', {'full_name': 'evil/kite'})]:
            self.run.clear(); self.run.update(copy.deepcopy(original)); self.run[key] = value
            calls = self.invoke()
            self.assertEqual(calls[0][0], 'workflow')

    def test_forged_identity_does_not_dispatch(self):
        original = copy.deepcopy(self.pr)
        for field, value in [('title', 'chore(player): release 0.7.0'), ('user', {'login': 'github-actions[bot]', 'type': 'User'}), ('state', 'closed')]:
            self.pr.clear(); self.pr.update(copy.deepcopy(original)); self.pr[field] = value
            self.assertEqual(self.invoke(), [])
        self.pr.clear(); self.pr.update(copy.deepcopy(original)); self.pr['head']['repo']['full_name'] = 'evil/kite'
        self.assertEqual(self.invoke(), [])

    def test_legacy_open_titles_never_approve_or_dispatch(self):
        for component, title in [('kite', 'chore(main): release 0.7.0'),
                                  ('player', 'chore(main): release player 0.7.0')]:
            self.pr['head']['ref'] = 'release-please--branches--main--components--' + component
            self.pr['title'] = title
            self.run['head_branch'] = self.pr['head']['ref']
            with self.subTest(component=component):
                self.state['runs'] = [self.run]
                self.assertEqual(self.invoke(), [])
                self.state['runs'] = []
                self.assertEqual(self.invoke(), [])

    def test_identity_races_refuse_all_writes(self):
        for field in ('sha', 'title', 'repo'):
            current = copy.deepcopy(self.pr)
            if field == 'sha': current['head']['sha'] = 'b' * 40
            elif field == 'title': current['title'] = 'chore(kite): release 9.9.9'
            else: current['head']['repo']['full_name'] = 'evil/kite'
            self.state['current'] = current
            self.state['runs'] = [self.run]
            self.assertEqual(self.invoke(1), [])
            self.state['runs'] = []
            self.assertEqual(self.invoke(1), [])

    def test_duplicate_pr_is_not_authorized(self):
        self.state['prs'].append(copy.deepcopy(self.pr))
        self.assertEqual(self.invoke(1), [])

unittest.main()
PY
