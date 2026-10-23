#!/usr/bin/env bash
# Exercise publisher entrypoints against fake GitHub, npm and registry boundaries.
# No subprocess or HTTP call in these tests can publish or mutate a real release.
set -euo pipefail
RELEASE_SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
export RELEASE_SCRIPT_DIR
python3 - <<'PY'
import base64
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch
from urllib.error import HTTPError, URLError
import zipfile

sys.path.insert(0, os.environ['RELEASE_SCRIPT_DIR'])
spec = importlib.util.spec_from_file_location('npm_release', os.environ['RELEASE_SCRIPT_DIR'] + '/npm-release.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
SHA = 'a' * 40


class Response(io.BytesIO):
    def __init__(self, data, url):
        super().__init__(data)
        self.url = url
    def geturl(self):
        return self.url


class NpmRelease(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        env = {'GITHUB_REPOSITORY': m.common.REPO, 'GITHUB_REF': 'refs/heads/main',
               'GITHUB_WORKFLOW_REF': m.common.REPO + '/.github/workflows/release-please.yml@refs/heads/main',
               'GITHUB_SHA': SHA, 'GITHUB_EVENT_NAME': 'push', 'TAG': 'player-v0.2.0',
               'RUNNER_TEMP': self.tmp.name, 'GITHUB_OUTPUT': self.tmp.name + '/out',
               'CI_POLL_TRIES': '1', 'CI_POLL_SLEEP': '0', 'NODE_AUTH_TOKEN': 'fake-token'}
        self.env = patch.dict(os.environ, env)
        self.env.start()
        self.addCleanup(self.env.stop)
        self.release = {'id': 1, 'tag_name': 'player-v0.2.0', 'draft': True, 'prerelease': False}
        self.commit = {'sha': SHA, 'parents': [{'sha': 'b' * 40}]}
        self.shadowed_tag = False
        self.pr = {'state': 'closed', 'merged_at': 'now', 'merge_commit_sha': SHA,
                   'user': {'login': 'github-actions[bot]', 'type': 'Bot'},
                   'base': {'ref': 'main', 'repo': {'full_name': m.common.REPO}},
                   'head': {'ref': 'release-please--branches--main--components--player', 'repo': {'full_name': m.common.REPO}},
                   'title': 'chore(main): release player 0.2.0'}
        self.run = {'id': 9, 'workflow_id': 5, 'event': 'push', 'head_sha': SHA,
                    'head_branch': 'main', 'head_repository': {'full_name': m.common.REPO},
                    'path': '.github/workflows/ci.yml',
                    'run_number': 4, 'run_attempt': 2, 'status': 'completed', 'conclusion': 'success'}
        self.jobs = [{'name': 'CI OK', 'conclusion': 'success', 'status': 'completed'}]
        self.artifact = {'id': 20, 'name': 'kite-player-package-2', 'expired': False,
                         'workflow_run': {'id': 9, 'head_sha': SHA}}
        self.package_name = m.NAME
        self.package_version = '0.2.0'
        self.member = 'package/package.json'
        self.member_type = tarfile.REGTYPE
        self.extra_zip = None
        self.manifest_changes = {}
        self.corrupt = False
        self.registry_status = 404
        self.registry_bytes = None
        self.registry_integrity = None
        self.registry_url = None
        self.patch_calls = []
        self.publish_calls = []
        self.requests = []
        self.artifacts_requested = False
        self.make_package()
        for target, handler in [('check_output', self.command), ('run', self.npm)]:
            mock = patch.object(m.subprocess, target, side_effect=handler)
            mock.start()
            self.addCleanup(mock.stop)
        mock = patch.object(m, 'urlopen', side_effect=self.http)
        mock.start()
        self.addCleanup(mock.stop)

    def make_package(self):
        stream = io.BytesIO()
        with tarfile.open(fileobj=stream, mode='w:gz') as archive:
            content = json.dumps({'name': self.package_name, 'version': self.package_version}).encode()
            info = tarfile.TarInfo(self.member)
            info.type = self.member_type
            info.linkname = '../../escape'
            info.size = len(content) if info.isfile() else 0
            archive.addfile(info, io.BytesIO(content) if info.isfile() else None)
        self.data = stream.getvalue()
        self.integrity = 'sha512-' + base64.b64encode(hashlib.sha512(self.data).digest()).decode()
        self.manifest = {'schemaVersion': 1, 'name': m.NAME, 'version': '0.2.0', 'sourceSha': SHA,
                         'runId': '9', 'runAttempt': 2, 'tarball': 'kite-player.tgz',
                         'sha256': hashlib.sha256(self.data).hexdigest(), 'integrity': self.integrity}
        self.manifest.update(self.manifest_changes)

    def archive(self):
        self.make_package()
        stream = io.BytesIO()
        with zipfile.ZipFile(stream, 'w') as archive:
            archive.writestr('manifest.json', json.dumps(self.manifest))
            archive.writestr('kite-player.tgz', self.data + (b'corrupt' if self.corrupt else b''))
            if self.extra_zip:
                archive.writestr(self.extra_zip, b'escape')
        return stream.getvalue()

    def command(self, args, **kwargs):
        self.assertEqual(args[0:2], ('gh', 'api'))
        paths = [arg for arg in args if arg.startswith('repos/')]
        self.assertEqual(len(paths), 1)
        path = paths[0].removeprefix('repos/' + m.common.REPO + '/')
        if path == 'actions/artifacts/20/zip':
            return self.archive()
        if '--method' in args:
            self.patch_calls.append((path, args))
            result = {}
        elif path.startswith('releases?'):
            result = [[self.release]]
        elif path.startswith('compare/'):
            result = {'status': 'identical'}
        elif '/pulls?' in path:
            result = [[self.pr]]
        elif path.startswith('commits/tags/') and self.shadowed_tag:
            result = dict(self.commit, sha='c' * 40)
        elif path.startswith('commits/'):
            result = self.commit
        elif path == 'actions/workflows/ci.yml':
            result = {'id': 5, 'path': '.github/workflows/ci.yml', 'state': 'active'}
        elif path.startswith('actions/workflows/5/runs?'):
            result = [{'workflow_runs': [self.run]}]
        elif path == 'actions/runs/9':
            result = self.run
        elif path.startswith('actions/runs/9/attempts/') and '/jobs?' in path:
            result = [{'jobs': self.jobs}]
        elif path == 'actions/runs/9/artifacts?per_page=100':
            self.artifacts_requested = True
            result = [{'artifacts': [self.artifact]}]
        else:
            raise AssertionError((args, kwargs))
        return json.dumps(result)

    def npm(self, args, **kwargs):
        self.assertEqual(args[:2], ('npm', 'publish'))
        self.assertTrue(kwargs['check'])
        self.publish_calls.append(args)
        self.registry_status = 200
        self.registry_bytes = Path(args[2]).read_bytes()

    def http(self, url, **kwargs):
        self.requests.append(url)
        if url.endswith('.tgz'):
            return Response(self.data if self.registry_bytes is None else self.registry_bytes, url)
        if self.registry_status != 200:
            raise HTTPError(url, self.registry_status, 'fixture error', {}, None)
        value = self.manifest['version']
        metadata = {'name': m.NAME, 'version': value, 'dist': {
            'tarball': self.registry_url or f'{m.REGISTRY}/{m.NAME}/-/kite-player-{value}.tgz',
            'integrity': self.registry_integrity or self.integrity}}
        return Response(json.dumps(metadata).encode(), url)

    def refused(self):
        with self.assertRaises((ValueError, m.subprocess.CalledProcessError)):
            m.prepare()
        self.assertEqual(self.publish_calls, [])
        self.assertEqual(self.patch_calls, [])

    def test_same_named_branch_cannot_substitute_for_the_release_tag(self):
        self.shadowed_tag = True
        # The branch points at an approved CI producer, but the actual protected
        # tag points elsewhere. That producer's package must never be published.
        self.refused()
        self.assertFalse(self.artifacts_requested)

    def test_exact_tested_bytes_are_published_and_public_release_is_not_latest(self):
        m.prepare()
        self.assertEqual(Path(os.environ['GITHUB_OUTPUT']).read_text(), 'publish=true\n')
        m.publish()
        self.assertEqual(len(self.publish_calls), 1)
        args = self.publish_calls[0]
        self.assertIn('--provenance', args)
        self.assertIn('--ignore-scripts', args)
        self.assertEqual(args[-2:], ('--tag', 'latest'))
        self.assertEqual(self.registry_bytes, self.data)
        m.finalize()
        self.assertEqual(len(self.patch_calls), 1)
        self.assertIn('draft=false', self.patch_calls[0][1])
        self.assertIn('make_latest=false', self.patch_calls[0][1])

    def test_prerelease_uses_next(self):
        os.environ['TAG'] = 'player-v0.3.0-rc.1'
        self.release.update(tag_name=os.environ['TAG'], prerelease=True)
        self.pr['title'] = 'chore(main): release player 0.3.0-rc.1'
        self.package_version = '0.3.0-rc.1'
        self.manifest_changes['version'] = self.package_version
        m.prepare()
        m.publish()
        self.assertEqual(self.publish_calls[0][-2:], ('--tag', 'next'))

    def test_failed_main_ci_never_downloads_or_publishes(self):
        self.run['conclusion'] = 'failure'
        self.refused()
        self.assertFalse(self.artifacts_requested)

    def test_failed_or_missing_ci_ok_is_refused(self):
        self.jobs = []
        self.refused()

    def test_ci_dispatch_is_not_a_push_producer(self):
        self.run['event'] = 'workflow_dispatch'
        self.refused()

    def test_wrong_ci_sha_is_refused(self):
        self.run['head_sha'] = 'b' * 40
        self.refused()

    def test_wrong_artifact_attempt_is_refused(self):
        self.artifact['name'] = 'kite-player-package-1'
        self.refused()

    def test_wrong_manifest_attempt_is_refused(self):
        self.manifest_changes['runAttempt'] = 1
        self.refused()

    def test_wrong_manifest_sha_is_refused(self):
        self.manifest_changes['sourceSha'] = 'b' * 40
        self.refused()

    def test_wrong_manifest_run_is_refused(self):
        self.manifest_changes['runId'] = '10'
        self.refused()

    def test_boolean_schema_version_is_not_integer_one(self):
        self.manifest_changes['schemaVersion'] = True
        self.refused()

    def test_expired_artifact_is_refused(self):
        self.artifact['expired'] = True
        self.refused()

    def test_wrong_artifact_source_is_refused(self):
        self.artifact['workflow_run']['head_sha'] = 'b' * 40
        self.refused()

    def test_tampered_tarball_is_refused(self):
        self.corrupt = True
        self.refused()

    def test_wrong_manifest_version_is_refused(self):
        self.manifest_changes['version'] = '8.0.0'
        self.refused()

    def test_wrong_embedded_version_is_refused_even_with_matching_hashes(self):
        self.package_version = '8.0.0'
        self.refused()

    def test_wrong_embedded_name_is_refused(self):
        self.package_name = '@attacker/kite-player'
        self.refused()

    def test_manifest_path_escape_is_refused(self):
        self.manifest_changes['tarball'] = '../../escape.tgz'
        self.refused()

    def test_zip_path_escape_is_refused_without_extraction(self):
        self.extra_zip = '../escape'
        self.refused()
        self.assertFalse((Path(self.tmp.name) / 'escape').exists())

    def test_tar_path_escape_is_refused_even_with_matching_hashes(self):
        self.member = 'package/../../escape'
        self.refused()

    def test_tar_symlink_is_refused(self):
        self.member_type = tarfile.SYMTYPE
        self.refused()

    def test_registry_server_failure_is_not_an_unpublished_version(self):
        self.registry_status = 503
        self.refused()

    def test_registry_auth_failure_is_not_an_unpublished_version(self):
        self.registry_status = 403
        self.refused()

    def test_registry_transport_error_never_enables_publication(self):
        with patch.object(m, 'urlopen', side_effect=URLError('offline')):
            with self.assertRaises(URLError):
                m.prepare()
        self.assertEqual(self.publish_calls, [])

    def test_existing_different_integrity_is_refused(self):
        self.registry_status = 200
        self.registry_integrity = 'sha512-different'
        self.refused()

    def test_existing_different_bytes_are_refused_even_if_registry_claims_matching_hash(self):
        self.registry_status = 200
        self.registry_bytes = b'different tarball'
        self.refused()

    def test_untrusted_registry_tarball_url_is_refused(self):
        self.registry_status = 200
        self.registry_url = 'https://attacker.example/kite-player.tgz'
        self.refused()
        self.assertFalse(any('attacker.example' in url for url in self.requests))

    def test_old_source_manual_recovery_only_finalizes_identical_published_bytes(self):
        os.environ['GITHUB_SHA'] = 'b' * 40
        os.environ['GITHUB_EVENT_NAME'] = 'workflow_dispatch'
        os.environ['GITHUB_WORKFLOW_REF'] = m.common.REPO + '/.github/workflows/npm.yml@refs/heads/main'
        self.registry_status = 200
        m.prepare()
        self.assertEqual(Path(os.environ['GITHUB_OUTPUT']).read_text(), 'publish=false\n')
        m.publish()
        m.finalize()
        self.assertEqual(self.publish_calls, [])
        self.assertEqual(len(self.patch_calls), 1)

    def test_old_source_manual_recovery_refuses_forged_provenance(self):
        os.environ['GITHUB_SHA'] = 'b' * 40
        os.environ['GITHUB_EVENT_NAME'] = 'workflow_dispatch'
        with self.assertRaisesRegex(ValueError, 'rerun the original'):
            m.prepare()
        self.assertEqual(self.publish_calls, [])

    def test_public_release_replay_does_not_patch_or_publish(self):
        self.registry_status = 200
        self.release['draft'] = False
        m.prepare()
        m.publish()
        m.finalize()
        self.assertEqual(self.patch_calls, [])
        self.assertEqual(self.publish_calls, [])

    def test_post_publish_registry_verification_precedes_public_release(self):
        m.prepare()
        m.publish()
        self.registry_bytes = b'tampered registry bytes'
        with self.assertRaises(ValueError):
            m.finalize()
        self.assertEqual(self.patch_calls, [])

    def test_missing_registry_version_cannot_make_github_release_public(self):
        m.prepare()
        with self.assertRaises(ValueError):
            m.finalize()
        self.assertEqual(self.patch_calls, [])

    def test_tampering_between_prepare_and_publish_is_refused(self):
        m.prepare()
        (Path(self.tmp.name) / 'kite-player-release' / 'kite-player.tgz').write_bytes(b'tamper')
        with self.assertRaises(ValueError):
            m.publish()
        self.assertEqual(self.publish_calls, [])

    def test_invalid_tags_are_rejected_before_remote_calls(self):
        for tag in ['v1.2.3', 'player-v01.2.3', 'player-v1.2.3;echo bad', '../player-v1.2.3',
                    'player-v1.2.3\n', 'player-v1.2.3-01']:
            with self.subTest(tag=tag):
                os.environ['TAG'] = tag
                self.refused()


unittest.main()
PY
