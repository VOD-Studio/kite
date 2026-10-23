#!/usr/bin/env bash
set -euo pipefail
PYTHONPATH="$(cd "$(dirname "$0")" && pwd)${PYTHONPATH:+:$PYTHONPATH}"
export PYTHONPATH
python3 - <<'PY'
import copy
import json
import os
import unittest
from unittest.mock import patch
import release_common as m

SHA = 'a' * 40

class Authorization(unittest.TestCase):
    def setUp(self):
        self.env = patch.dict(os.environ, {'GITHUB_REPOSITORY': m.REPO, 'GITHUB_REF': 'refs/heads/main', 'GITHUB_WORKFLOW_REF': m.REPO + '/.github/workflows/release.yml@refs/heads/main', 'CI_POLL_TRIES': '1'})
        self.env.start(); self.addCleanup(self.env.stop)
        self.commit = {'sha': SHA, 'parents': [{'sha': 'b' * 40}]}
        self.pr = {'state': 'closed', 'merged_at': 'now', 'merge_commit_sha': SHA, 'user': {'login': 'github-actions[bot]', 'type': 'Bot'}, 'base': {'ref': 'main', 'repo': {'full_name': m.REPO}}, 'head': {'ref': m.release_branch('kite'), 'repo': {'full_name': m.REPO}}, 'title': 'chore(kite): release 0.7.0'}
        self.run = {'id': 9, 'workflow_id': 5, 'path': '.github/workflows/ci.yml', 'event': 'push', 'head_sha': SHA, 'head_branch': 'main', 'head_repository': {'full_name': m.REPO}, 'run_number': 1, 'run_attempt': 2, 'status': 'completed', 'conclusion': 'success'}
        self.workflow = {'id': 5, 'path': '.github/workflows/ci.yml', 'state': 'active'}
        self.jobs = [{'name': 'CI OK', 'status': 'completed', 'conclusion': 'success'}]
        self.status = 'ahead'
        self.listed = [{'tag_name': 'v0.7.0', 'draft': True}]
        self.prs = [self.pr]
        self.runs = [self.run]
        self.detail = None
        self.calls = []
        mock = patch.object(m, 'command', side_effect=self.command)
        mock.start(); self.addCleanup(mock.stop)

    def command(self, *args):
        self.calls.append(args)
        path = args[-1].removeprefix('repos/' + m.REPO + '/')
        if path.startswith('commits/') and '/pulls?' in path: value = [self.prs]
        elif path.startswith('commits/'): value = self.commit
        elif path.startswith('compare/'): value = {'status': self.status}
        elif path == 'actions/workflows/ci.yml': value = self.workflow
        elif path.startswith('actions/workflows/5/runs?'): value = [{'workflow_runs': self.runs}]
        elif '/attempts/' in path:
            self.assertIn('/attempts/2/jobs?', path)
            value = [{'jobs': self.jobs}]
        elif path == 'actions/runs/9': value = self.detail or self.run
        elif path == 'releases?per_page=100': value = [self.listed]
        else: raise AssertionError(args)
        return json.dumps(value)

    def test_both_component_identities(self):
        for component, title in [('kite', 'chore(kite): release 0.7.0'), ('player', 'chore(player): release 0.7.0')]:
            self.pr['head']['ref'] = m.release_branch(component)
            self.pr['title'] = title
            m.require_release_pr(self.commit, component, '0.7.0')
            other = 'player' if component == 'kite' else 'kite'
            with self.assertRaises(ValueError): m.require_release_pr(self.commit, other, '0.7.0')

    def test_open_titles_require_explicit_product_and_exact_version(self):
        self.pr.update(state='open', merged_at=None, merge_commit_sha=None)
        for component in m.COMPONENTS:
            self.pr['head']['ref'] = m.release_branch(component)
            self.pr['title'] = f'chore({component}): release 0.7.0'
            with self.subTest(component=component):
                self.assertTrue(m.trusted_release_pr(self.pr, component, '0.7.0'))
                self.assertFalse(m.trusted_release_pr(self.pr, component, '0.7.1'))
                self.assertFalse(m.trusted_merged_release_pr(self.pr, component, '0.7.0'))
                other = 'player' if component == 'kite' else 'kite'
                self.assertFalse(m.trusted_release_pr(self.pr, other, '0.7.0'))
                self.pr['title'] = 'chore(main): release ' + ('player ' if component == 'player' else '') + '0.7.0'
                self.assertFalse(m.trusted_release_pr(self.pr, component))
                self.assertFalse(m.trusted_merged_release_pr(self.pr, component))

    def test_historical_titles_only_authorize_merged_release_reads(self):
        original = copy.deepcopy(self.pr)
        for component in m.COMPONENTS:
            self.pr.clear(); self.pr.update(copy.deepcopy(original))
            self.pr['head']['ref'] = m.release_branch(component)
            self.pr['title'] = 'chore(main): release ' + ('player ' if component == 'player' else '') + '0.7.0'
            with self.subTest(component=component):
                self.assertFalse(m.trusted_release_pr(self.pr, component))
                m.require_release_pr(self.commit, component, '0.7.0')
                with self.assertRaises(ValueError): m.require_release_pr(self.commit, component, '0.7.1')
                other = 'player' if component == 'kite' else 'kite'
                with self.assertRaises(ValueError): m.require_release_pr(self.commit, other, '0.7.0')
            historical = copy.deepcopy(self.pr)
            for field, value in [('state', 'open'), ('merged_at', None), ('merge_commit_sha', 'c' * 40),
                                 ('user', {'login': 'human', 'type': 'Bot'}),
                                 ('head', {'ref': m.release_branch(other), 'repo': {'full_name': m.REPO}}),
                                 ('base', {'ref': 'main', 'repo': {'full_name': 'evil/kite'}})]:
                self.pr.clear(); self.pr.update(copy.deepcopy(historical)); self.pr[field] = value
                with self.subTest(component=component, field=field), self.assertRaises(ValueError):
                    m.require_release_pr(self.commit, component, '0.7.0')

    def test_forged_release_identity(self):
        original = copy.deepcopy(self.pr)
        changes = [('user', 'login', 'human'), ('user', 'type', 'User'), ('base', 'ref', 'other'), ('head', 'ref', 'release-please--branches--main')]
        for section, key, value in changes:
            self.pr.clear(); self.pr.update(copy.deepcopy(original)); self.pr[section][key] = value
            with self.subTest(section=section, key=key), self.assertRaises(ValueError): m.require_release_pr(self.commit, 'kite', '0.7.0')
        for section in ('head', 'base'):
            self.pr.clear(); self.pr.update(copy.deepcopy(original)); self.pr[section]['repo']['full_name'] = 'evil/kite'
            with self.subTest(section=section), self.assertRaises(ValueError): m.require_release_pr(self.commit, 'kite', '0.7.0')
        for field, value in [('title', 'chore(player): release 0.7.0'), ('title', 'chore(kite): release 0.7.0\n'), ('title', 'chore(kite): release 0.7.1'), ('merged_at', None), ('merge_commit_sha', 'c' * 40)]:
            self.pr.clear(); self.pr.update(copy.deepcopy(original)); self.pr[field] = value
            with self.subTest(field=field, value=value), self.assertRaises(ValueError): m.require_release_pr(self.commit, 'kite', '0.7.0')

    def test_squash_and_unique_pr_required(self):
        self.commit['parents'].append({'sha': 'c' * 40})
        with self.assertRaises(ValueError): m.require_release_pr(self.commit, 'kite', '0.7.0')
        self.commit['parents'].pop()
        self.prs.append(copy.deepcopy(self.pr))
        with self.assertRaises(ValueError): m.require_release_pr(self.commit, 'kite', '0.7.0')

    def test_exact_main_source(self):
        self.assertEqual(m.resolve_source('v0.7.0')['sha'], SHA)
        for status in ('behind', 'diverged', 'unknown'):
            self.status = status
            with self.assertRaises(ValueError): m.resolve_source('v0.7.0')

    def test_trusted_workflow_context(self):
        m.require_main()
        for key, value in [('GITHUB_REPOSITORY', 'evil/kite'), ('GITHUB_REF', 'refs/tags/v0.7.0'), ('GITHUB_WORKFLOW_REF', m.REPO + '/.github/workflows/release.yml@refs/heads/evil')]:
            with patch.dict(os.environ, {key: value}), self.assertRaises(ValueError): m.require_main()

    def test_current_attempt_success(self):
        self.assertEqual(m.wait_main_ci(SHA)['run_attempt'], 2)
        self.assertTrue(any('/attempts/2/jobs?' in call[-1] for call in self.calls))

    def test_ci_wrong_identity(self):
        original = copy.deepcopy(self.run)
        for key, value in [('workflow_id', 8), ('path', '.github/workflows/ai.yml'), ('event', 'workflow_dispatch'), ('head_sha', 'c' * 40), ('head_branch', 'other'), ('head_repository', {'full_name': 'evil/kite'})]:
            self.run.clear(); self.run.update(copy.deepcopy(original)); self.run[key] = value
            with self.subTest(key=key), self.assertRaises(ValueError): m.wait_main_ci(SHA)

    def test_gate_fail_closed(self):
        for conclusion in ('failure', 'cancelled', 'skipped', 'neutral', None):
            self.jobs[0]['conclusion'] = conclusion
            with self.subTest(conclusion=conclusion), self.assertRaises(ValueError): m.wait_main_ci(SHA)
        self.jobs = []
        with self.assertRaises(ValueError): m.wait_main_ci(SHA)
        self.jobs = [{'name': 'CI OK', 'status': 'completed', 'conclusion': 'success'}] * 2
        with self.assertRaises(ValueError): m.wait_main_ci(SHA)

    def test_old_success_cannot_hide_failed_new_attempt(self):
        self.detail = dict(self.run, run_attempt=3, conclusion='failure')
        with self.assertRaises(ValueError): m.wait_main_ci(SHA)
        self.detail = dict(self.run, run_attempt=1)
        with self.assertRaises(ValueError): m.wait_main_ci(SHA)

    def test_unknown_cancelled_and_missing_producer(self):
        for status, conclusion in [('completed', 'cancelled'), ('completed', None), ('unknown', None)]:
            self.run.update(status=status, conclusion=conclusion)
            with self.assertRaises(ValueError): m.wait_main_ci(SHA)
        self.runs = []
        with self.assertRaises(ValueError): m.wait_main_ci(SHA)

    def test_draft_listing_and_ambiguity(self):
        self.assertTrue(m.release_for('v0.7.0')['draft'])
        self.listed *= 2
        with self.assertRaises(ValueError): m.release_for('v0.7.0')
        self.listed = []
        with self.assertRaises(ValueError): m.release_for('v0.7.0')

    def test_component_tags(self):
        self.assertEqual(m.parse_tag('v1.2.3', 'kite'), '1.2.3')
        self.assertEqual(m.parse_tag('player-v1.2.3-rc.1', 'player'), '1.2.3-rc.1')
        for component, tags in [('kite', ['player-v1.2.3', 'v1.2.3-rc.1', 'v01.2.3', 'v1.2.3\n']), ('player', ['v1.2.3', 'player-v1.2.3-01', 'player-v1.2.3+meta'])]:
            for tag in tags:
                with self.subTest(tag=tag), self.assertRaises(ValueError): m.parse_tag(tag, component)

unittest.main()
PY
