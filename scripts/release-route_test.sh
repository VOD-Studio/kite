#!/usr/bin/env bash
# Exercise the actual router process with only its external GitHub CLI replaced.
set -euo pipefail
RELEASE_SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
export RELEASE_SCRIPT_DIR
python3 - <<'PY'
import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

REPO = 'VOD-Studio/kite'
A, B, C = ('a' * 40, 'b' * 40, 'c' * 40)


def release_pr(component='player', version='0.2.0', sha=B):
    return {'number': 23, 'state': 'closed', 'merged_at': '2026-10-22T00:00:00Z',
            'merge_commit_sha': sha, 'user': {'login': 'github-actions[bot]', 'type': 'Bot'},
            'base': {'ref': 'main', 'repo': {'full_name': REPO}},
            'head': {'ref': 'release-please--branches--main--components--' + component,
                     'repo': {'full_name': REPO}},
            'title': f'chore({component}): release {version}',
            'labels': [{'name': 'autorelease: tagged'}]}


# This models REST resources, not router functions or a list of expected actions.
# Unknown requests and any write method fail at the external process boundary.
GH = '''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
if not args or args[0] != 'api' or any(x in args for x in ('-X', '--method', '-f', '-F', '--input')):
    sys.exit('unexpected GitHub command or remote write')
state = json.loads(Path(os.environ['GH_STATE']).read_text())
route = args[-1].removeprefix('repos/VOD-Studio/kite/')
with open(os.environ['GH_CALLS'], 'a') as calls:
    calls.write(route + '\\n')
if route.startswith('commits/') and '/pulls?' in route:
    sha = route.split('/')[1]
    value = state['prs'].get(sha, [])
elif route == 'releases?per_page=100':
    value = state['releases']
elif route.startswith('commits/tags/'):
    tag = route.removeprefix('commits/tags/')
    sha = state['tags'].get(tag)
    if sha is None: sys.exit('tag not found')
    value = state['commits'][sha]
elif route.startswith('commits/'):
    ref = route.removeprefix('commits/')
    sha = state.get('ambiguous_refs', {}).get(ref, state['tags'].get(ref, ref))
    if sha not in state['commits']: sys.exit('commit not found')
    value = state['commits'][sha]
elif route.startswith('compare/'):
    sha = route.removeprefix('compare/').split('...')[0]
    status = 'identical' if sha == state['main'] else 'ahead' if sha in state['ancestors'] else 'diverged'
    value = {'status': status}
else:
    sys.exit('unexpected endpoint: ' + route)
if '--slurp' in args: value = [value]
print(json.dumps(value))
'''


class ReleaseRoute(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.gh = self.root / 'gh'
        self.gh.write_text(GH)
        self.gh.chmod(0o755)
        self.state = {'prs': {B: [release_pr()]},
                      'releases': [{'id': 8, 'tag_name': 'player-v0.2.0', 'draft': True, 'prerelease': False}],
                      'tags': {'player-v0.2.0': B}, 'main': B, 'ancestors': [A, B],
                      'commits': {sha: {'sha': sha, 'parents': [{'sha': A}]} for sha in (A, B, C)}}

    def route(self, sha=B, env=None, success=True):
        (self.root / 'state.json').write_text(json.dumps(self.state))
        out = self.root / 'output'
        out.write_text('')
        calls = self.root / 'calls'
        calls.write_text('')
        settings = dict(os.environ, GH=str(self.gh), GH_STATE=str(self.root / 'state.json'),
                        GH_CALLS=str(calls), GITHUB_OUTPUT=str(out), GITHUB_SHA=sha,
                        GITHUB_REPOSITORY=REPO, GITHUB_REF='refs/heads/main',
                        GITHUB_WORKFLOW_REF=REPO + '/.github/workflows/release-please.yml@refs/heads/main',
                        GITHUB_EVENT_NAME='push')
        settings.update(env or {})
        result = subprocess.run([sys.executable, os.environ['RELEASE_SCRIPT_DIR'] + '/release-route.py'],
                                env=settings, text=True, capture_output=True)
        if success:
            self.assertEqual(result.returncode, 0, result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout)
            self.assertEqual(out.read_text(), '', 'failed authorization must emit no ready outputs')
        return dict(line.split('=', 1) for line in out.read_text().splitlines()), calls.read_text().splitlines()

    def test_queued_a_cannot_claim_b_b_recovers_without_created_output(self):
        # A's release-please action already created B's draft and removed pending.
        # Its action output is deliberately not an input to the router.
        outputs, calls = self.route(A)
        self.assertEqual(outputs, {'go_tag': '', 'player_tag': ''})
        self.assertFalse(any(route.startswith('releases') for route in calls))
        outputs, calls = self.route(B)
        self.assertEqual(outputs, {'go_tag': '', 'player_tag': 'player-v0.2.0'})
        self.assertIn('commits/' + B + '/pulls?per_page=100', calls)
        self.assertNotIn('commits/' + A + '/pulls?per_page=100', calls)

    def test_main_advancing_to_c_does_not_change_b_source(self):
        self.state.update(main=C, ancestors=[A, B, C])
        outputs, calls = self.route(B)
        self.assertEqual(outputs['player_tag'], 'player-v0.2.0')
        self.assertIn('compare/' + B + '...main', calls)
        outputs, _ = self.route(C, {'GITHUB_EVENT_NAME': 'workflow_dispatch'})
        self.assertEqual(outputs, {'go_tag': '', 'player_tag': ''})

    def test_ordinary_business_pr_has_no_release_route(self):
        self.state['prs'][B] = [dict(release_pr(), title='feat(player): add controls',
                                    head={'ref': 'feature/controls', 'repo': {'full_name': REPO}},
                                    user={'login': 'maintainer', 'type': 'User'})]
        outputs, _ = self.route()
        self.assertEqual(outputs, {'go_tag': '', 'player_tag': ''})

    def test_go_and_player_route_only_their_product(self):
        self.state['prs'][B] = [release_pr('kite')]
        self.state['tags']['v0.2.0'] = B
        self.state['releases'].append({'id': 9, 'tag_name': 'v0.2.0', 'draft': True, 'prerelease': False})
        outputs, _ = self.route()
        self.assertEqual(outputs, {'go_tag': 'v0.2.0', 'player_tag': ''})

    def test_public_release_replay_is_read_only_and_still_source_owned(self):
        for component, tag in [('player', 'player-v0.2.0'), ('kite', 'v0.2.0')]:
            with self.subTest(component=component):
                self.state['prs'][B] = [release_pr(component)]
                self.state['tags'] = {tag: B}
                self.state['releases'] = [{'id': 8, 'tag_name': tag, 'draft': False, 'prerelease': False}]
                outputs, _ = self.route()
                self.assertEqual(outputs, {'go_tag': tag if component == 'kite' else '',
                                           'player_tag': tag if component == 'player' else ''})
                outputs, _ = self.route(A)
                self.assertEqual(outputs, {'go_tag': '', 'player_tag': ''})

    def test_historical_merged_titles_still_route_exact_product(self):
        for component, tag, title in [('kite', 'v0.2.0', 'chore(main): release 0.2.0'),
                                      ('player', 'player-v0.2.0', 'chore(main): release player 0.2.0')]:
            with self.subTest(component=component):
                pr = dict(release_pr(component), title=title)
                self.state['prs'][B] = [pr]
                self.state['tags'] = {tag: B}
                self.state['releases'] = [{'id': 8, 'tag_name': tag, 'draft': True}]
                outputs, _ = self.route()
                self.assertEqual(outputs, {'go_tag': tag if component == 'kite' else '',
                                           'player_tag': tag if component == 'player' else ''})
                pr['state'] = 'open'
                self.route(success=False)

    def test_duplicate_or_cross_product_release_prs_fail_closed(self):
        for second in [release_pr(), release_pr('kite')]:
            with self.subTest(second=second['title']):
                self.state['prs'][B] = [release_pr(), second]
                self.route(success=False)

    def test_forged_and_mismatched_release_prs_fail_closed(self):
        original = release_pr()
        mutations = [('user', 'login', 'human'), ('user', 'type', 'User'),
                     ('base', 'ref', 'other'), ('head', 'ref', 'feature/forged')]
        for section, field, value in mutations:
            pr = copy.deepcopy(original)
            pr[section][field] = value
            self.state['prs'][B] = [pr]
            with self.subTest(section=section, field=field): self.route(success=False)
        for section in ['base', 'head']:
            pr = copy.deepcopy(original)
            pr[section]['repo']['full_name'] = 'evil/kite'
            self.state['prs'][B] = [pr]
            with self.subTest(section=section): self.route(success=False)
        for field, value in [('title', 'chore(kite): release 0.2.0'),
                             ('title', 'chore(player): release 01.2.0'),
                             ('merge_commit_sha', A), ('merged_at', None), ('state', 'open')]:
            self.state['prs'][B] = [dict(original, **{field: value})]
            with self.subTest(field=field, value=value): self.route(success=False)

    def test_missing_duplicate_and_wrong_version_releases_fail(self):
        original = copy.deepcopy(self.state['releases'])
        for releases in [[], original * 2, [dict(original[0], tag_name='player-v0.3.0')],
                         [dict(original[0], tag_name='v0.2.0')]]:
            self.state['releases'] = releases
            with self.subTest(releases=releases): self.route(success=False)

    def test_wrong_actual_tag_sha_and_ambiguous_branch_fail(self):
        self.state['tags']['player-v0.2.0'] = A
        self.route(success=False)
        self.state['ambiguous_refs'] = {'player-v0.2.0': B}
        self.route(success=False)

    def test_missing_tag_non_main_and_non_squash_sources_fail(self):
        original = copy.deepcopy(self.state)
        self.state['tags'] = {}
        self.route(success=False)
        self.state = copy.deepcopy(original)
        self.state.update(main=C, ancestors=[C])
        self.route(success=False)
        self.state = copy.deepcopy(original)
        self.state['commits'][B]['parents'].append({'sha': C})
        self.route(success=False)

    def test_pr_query_is_bounded_and_rejects_truncation(self):
        self.state['prs'][B] = [dict(release_pr(), title='feat: ordinary', head={'ref': 'feature'})] * 100
        _, calls = self.route(success=False)
        self.assertEqual(calls, ['commits/' + B + '/pulls?per_page=100'])

    def test_untrusted_workflow_and_tag_refs_fail_before_api(self):
        for env in [{'GITHUB_REF': 'refs/tags/player-v0.2.0'},
                    {'GITHUB_REPOSITORY': 'evil/kite'},
                    {'GITHUB_WORKFLOW_REF': REPO + '/.github/workflows/release-please.yml@refs/heads/evil'},
                    {'GITHUB_SHA': 'main'}]:
            with self.subTest(env=env):
                _, calls = self.route(env=env, success=False)
                self.assertEqual(calls, [])


unittest.main()
PY
