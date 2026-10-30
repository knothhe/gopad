import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('go_wrapper', ROOT / 'scripts/go.py')
wrapper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(wrapper)


class GoOverlayTests(unittest.TestCase):
    def test_patch_requires_exact_source(self):
        with self.assertRaisesRegex(ValueError, 'source changed'):
            wrapper.patched_source('changed upstream')
        text = 'package mygo\nimport (\n\t"github.com/egoist/mygo/internal/update"\n)\nfunc extract() {\n\terr = update.ExtractArchive(f, unpacked)\n}\n'
        with mock.patch.object(wrapper, 'UPDATER_SHA256', hashlib.sha256(text.encode()).hexdigest()):
            patched = wrapper.patched_source(text)
            self.assertIn('updatearchive.ExtractZIP(f, unpacked)', patched)
            self.assertIn('err = update.ExtractArchive(f, unpacked)', patched)
            self.assertIn('github.com/knothhe/gopad/pkg/updatearchive', patched)

    def test_overlay_conflicts_fail_before_running_go(self):
        for flags in ['-overlay=old.json', '-modfile old.mod']:
            with self.subTest(flags=flags), mock.patch.dict(os.environ, {'GOFLAGS': flags}), mock.patch.object(wrapper.sys, 'argv', ['go.py', 'test', './...']), mock.patch.object(wrapper.subprocess, 'run') as run:
                with self.assertRaisesRegex(ValueError, 'conflicts'):
                    wrapper.main()
                run.assert_not_called()

    def test_cold_cache_download_and_private_copy(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / 'mygo.json').write_text('{}')
            (root / 'go.mod').write_text('module fixture\n')
            (root / 'go.sum').write_text('fixture sums\n')
            original = root / 'cache/mygo'
            original.mkdir(parents=True)
            source = 'package mygo\n'
            (original / 'updater.go').write_text(source)
            responses = [subprocess.CompletedProcess([], 0, json.dumps({'Version': wrapper.MYGO_VERSION})), subprocess.CompletedProcess([], 0, json.dumps({'Version': wrapper.MYGO_VERSION, 'Dir': str(original)}))]
            def build(args, cwd, env):
                import shlex
                flags = shlex.split(env['GOFLAGS'])
                overlay = Path(next(flag.split('=', 1)[1] for flag in flags if flag.startswith('-overlay=')))
                mapping = json.loads(overlay.read_text())['Replace']
                copied, patch = next(iter(mapping.items()))
                self.assertNotEqual(Path(copied).parent, original)
                self.assertEqual(Path(copied).read_text(), source)
                self.assertEqual(Path(patch).read_text(), 'patched')
                modfile = Path(next(flag.split('=', 1)[1] for flag in flags if flag.startswith('-modfile=')))
                self.assertIn('replace github.com/egoist/mygo => ', modfile.read_text())
                self.assertEqual(modfile.with_suffix('.sum').read_text(), 'fixture sums\n')
                return 0
            with mock.patch.object(wrapper, 'ROOT', root), mock.patch.object(wrapper, 'patched_source', return_value='patched'), mock.patch.dict(os.environ, {'GOFLAGS': ''}), mock.patch.object(wrapper.sys, 'argv', ['go.py', 'test', './...']), mock.patch.object(wrapper.subprocess, 'run', side_effect=responses) as run, mock.patch.object(wrapper.subprocess, 'call', side_effect=build):
                self.assertEqual(wrapper.main(), 0)
                self.assertEqual(run.call_args_list[1].args[0][:4], ['go', 'mod', 'download', '-json'])
            self.assertEqual((original / 'updater.go').read_text(), source)
            self.assertEqual(list((root / 'build').iterdir()), [])


if __name__ == '__main__':
    unittest.main()
