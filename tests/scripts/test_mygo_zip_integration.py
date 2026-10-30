"""Exercise signed ZIPs through the actual, patched MyGo macOS installer."""
import importlib.util
import json
from pathlib import Path
import shlex
import subprocess
import sys
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('gopad_go_zip_integration', ROOT / 'scripts/go.py')
GO = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(GO)


@unittest.skipUnless(sys.platform == 'darwin', 'MyGo exit-time bundle staging is macOS specific')
class MyGoZIPIntegration(unittest.TestCase):
    def test_real_framework_download_signature_staging_and_install(self):
        original_call = subprocess.call

        def run_with_framework_test(arguments, **kwargs):
            flags = shlex.split(kwargs['env']['GOFLAGS'])
            overlay = Path(next(flag.split('=', 1)[1] for flag in flags if flag.startswith('-overlay=')))
            mapping = json.loads(overlay.read_text())
            module = Path(next(iter(mapping['Replace']))).parent
            mapping['Replace'][str(module / 'gopad_zip_integration_test.go')] = str(
                ROOT / 'scripts/testdata/mygo_zip_integration_test.go')
            overlay.write_text(json.dumps(mapping))
            return original_call(arguments, **kwargs)

        # Include MyGo's existing tar archive test to verify the fallback remains
        # compatible while the new tests exercise ZIP through the same installer.
        args = ['go.py', 'test', '-race', 'github.com/egoist/mygo', '-run',
                '^(TestGopadSignedZIP|TestUpdater$)', '-count=1', '-timeout=90s']
        with mock.patch.object(sys, 'argv', args), mock.patch.object(
                GO.subprocess, 'call', side_effect=run_with_framework_test):
            self.assertEqual(GO.main(), 0)


if __name__ == '__main__':
    unittest.main()
