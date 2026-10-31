"""Exercise release publication when list responses lag behind mutations."""
import contextlib
import copy
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'scripts'))
SPEC = importlib.util.spec_from_file_location('publish_release', ROOT / 'scripts/publish-release.py')
PUBLISH = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PUBLISH)


class PublishReleaseTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.version = '0.1.2'
        self.remote = None
        self.listed = None
        self.assets = {}
        self.downloads = []
        self.calls = []
        self.notes = 'Release notes with Unicode: 中文\n\nSecond paragraph.'
        self.directory = self.root / 'build/release/darwin-arm64'
        self.directory.mkdir(parents=True)

    def prepare(self):
        self.contents = {
            f'gopad-{self.version}-macos-arm64.zip': b'signed ZIP fixture',
            'update-darwin-arm64.json': b'signed manifest fixture',
            'SHA256SUMS': b'checksum fixture',
        }
        for name, content in self.contents.items():
            (self.directory / name).write_bytes(content)

    def release(self, draft=True):
        return {'id': 123, 'tag_name': 'v' + self.version, 'draft': draft,
                'assets': list(self.assets.values()), 'html_url': 'https://example.invalid/release'}

    def add_asset(self, name, content):
        self.assets[name] = {'id': len(self.assets) + 1, 'name': name, 'size': len(content)}

    def gh(self, *args):
        self.calls.append(args)
        if args[:3] == ('api', '--paginate', '--slurp'):
            if '/assets?' in args[3]:
                return json.dumps([list(self.assets.values())])
            # Deliberately never refresh the release list after a mutation.
            return json.dumps([[self.listed] if self.listed else []])
        if args[:3] == ('api', '--method', 'POST'):
            self.assertEqual(args[3], 'repos/fixture/gopad/releases')
            self.assertIn('tag_name=v' + self.version, args)
            self.assertIn('target_commitish=fixture-commit', args)
            self.assertIn('body=' + self.notes, args)
            self.assertIn('draft=true', args)
            self.assertIn('prerelease=' + str('-' in self.version.split('+')[0]).lower(), args)
            self.remote = self.release()
            return json.dumps(self.remote)
        if args[:3] == ('api', '--method', 'PATCH'):
            self.assertEqual(args[3:], ('repos/fixture/gopad/releases/123', '-F', 'draft=false'))
            self.remote = self.release(draft=False)
            return json.dumps(self.remote)
        if args[:2] == ('release', 'upload'):
            name = Path(args[3]).name
            self.assertNotIn(name, self.assets, 'Existing assets must never be replaced')
            self.add_asset(name, Path(args[3]).read_bytes())
            return ''
        if args == ('api', 'repos/fixture/gopad/git/ref/tags/v' + self.version):
            return '{}'
        self.fail(f'Unexpected gh call: {args}')

    def download_asset(self, args, **kwargs):
        asset_id = int(args[-1].rsplit('/', 1)[1])
        name = next(name for name, asset in self.assets.items() if asset['id'] == asset_id)
        return subprocess.CompletedProcess(args, 0, self.contents[name])

    def public_download(self, url, **kwargs):
        self.downloads.append(url)
        return io.BytesIO(self.contents[url.rsplit('/', 1)[1]])

    def publish(self):
        environment = {'GITHUB_ACTIONS': 'true', 'GITHUB_REPOSITORY': 'fixture/gopad',
                       'RELEASE_TAG': 'v' + self.version}
        with mock.patch.dict(os.environ, environment, clear=True), \
                mock.patch.object(PUBLISH, 'ROOT', self.root), \
                mock.patch.object(PUBLISH, 'version', return_value=self.version), \
                mock.patch.object(PUBLISH, 'run', side_effect=['', 'fixture-commit', '', self.notes]), \
                mock.patch.object(PUBLISH, 'gh', side_effect=self.gh), \
                mock.patch.object(PUBLISH.subprocess, 'run', side_effect=self.download_asset), \
                mock.patch.object(PUBLISH.urllib.request, 'urlopen', side_effect=self.public_download), \
                contextlib.redirect_stdout(io.StringIO()):
            PUBLISH.main()

    def test_create_and_publish_with_stale_release_list(self):
        self.prepare()
        self.publish()
        self.assertFalse(self.remote['draft'])
        self.assertEqual(set(self.assets), set(self.contents))
        self.assertEqual(len(self.downloads), 3)
        listings = [args for args in self.calls if args[:3] == ('api', '--paginate', '--slurp')
                    and '/assets?' not in args[3]]
        self.assertEqual(len(listings), 1)

    def test_prerelease_and_build_metadata(self):
        for version in ('0.1.3-rc.1', '0.1.3+build-1'):
            with self.subTest(version=version):
                self.version = version
                self.assets = {}
                self.prepare()
                self.publish()
                self.assertFalse(self.remote['draft'])

    def test_resume_partial_draft_without_replacing_assets(self):
        self.prepare()
        self.add_asset('SHA256SUMS', self.contents['SHA256SUMS'])
        self.listed = self.remote = copy.deepcopy(self.release())
        self.publish()
        self.assertFalse(self.remote['draft'])
        self.assertEqual(sum(args[:2] == ('release', 'upload') for args in self.calls), 2)
        self.assertFalse(any(args[:3] == ('api', '--method', 'POST') for args in self.calls))

    def test_published_release_is_verified_without_mutations(self):
        self.prepare()
        for name, content in self.contents.items():
            self.add_asset(name, content)
        self.listed = self.remote = self.release(draft=False)
        self.publish()
        self.assertTrue(all(args[0] == 'api' and '--method' not in args for args in self.calls))
        self.assertEqual(self.downloads, [])

    def test_mismatched_remote_content_prevents_publication(self):
        self.prepare()
        for name, content in self.contents.items():
            self.add_asset(name, content)
        self.listed = self.remote = self.release()
        self.contents['SHA256SUMS'] = self.contents['SHA256SUMS'].upper()
        with self.assertRaisesRegex(PUBLISH.BuildError, 'content mismatch'):
            self.publish()
        self.assertTrue(self.remote['draft'])
        self.assertFalse(any('--method' in args for args in self.calls))

    def test_missing_remote_tag_prevents_creation(self):
        self.prepare()
        original = self.gh

        def missing_tag(*args):
            if args == ('api', 'repos/fixture/gopad/git/ref/tags/v' + self.version):
                raise PUBLISH.BuildError('Remote tag missing')
            return original(*args)

        with mock.patch.object(self, 'gh', side_effect=missing_tag):
            with self.assertRaisesRegex(PUBLISH.BuildError, 'Remote tag missing'):
                self.publish()
        self.assertIsNone(self.remote)


if __name__ == '__main__':
    unittest.main()
