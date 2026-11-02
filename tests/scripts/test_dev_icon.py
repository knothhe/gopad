import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import struct
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('dev_icon', ROOT / 'scripts/dev_icon.py')
icon = importlib.util.module_from_spec(spec)
spec.loader.exec_module(icon)


class DevIconOptionsTests(unittest.TestCase):
    def test_upscaling_requires_explicit_opt_in(self):
        args = icon.parser().parse_args(['--input', 'source.png', '--output-dir', 'output', '--formats', 'icns'])
        with self.assertRaisesRegex(icon.IconError, 'allow-upscale'):
            icon.parse_options(args, 256)
        args.allow_upscale = True
        self.assertEqual(max(icon.parse_options(args, 256)[2]), 1024)


@unittest.skipUnless(sys.platform == 'darwin' and shutil.which('xcrun'), 'Requires macOS native rendering tools')
class NativeDevIconTests(unittest.TestCase):
    def test_generation_without_python_packages_and_native_container_roundtrip(self):
        original = (ROOT / 'assets/gopad.png').read_bytes()
        with tempfile.TemporaryDirectory(prefix='gopad-icon-test-') as temp:
            directory = Path(temp)
            output = directory / 'dev icons'
            command = [sys.executable, '-S', str(ROOT / 'scripts/dev_icon.py'),
                       '--input', str(ROOT / 'assets/gopad.png'), '--output-dir', str(output),
                       '--name', 'gopad-dev', '--formats', 'png,ico,icns']
            subprocess.run(command, check=True, capture_output=True, text=True)
            record = json.loads((output / icon.MANIFEST).read_text())
            for name, digest in record['files'].items():
                self.assertEqual(hashlib.sha256((output / name).read_bytes()).hexdigest(), digest)
            for size in icon.PNG_SIZES:
                data = (output / f'gopad-dev-{size}.png').read_bytes()
                self.assertEqual(struct.unpack_from('>II', data, 16), (size, size))
            # Both native decoders and iconutil must accept the generated containers.
            for suffix, largest in [('ico', 256), ('icns', 1024)]:
                sizes = json.loads(icon.native_renderer('--inspect', output / f'gopad-dev.{suffix}'))
                self.assertEqual(max(sizes), largest)
            iconset = directory / 'roundtrip.iconset'
            subprocess.run(['iconutil', '-c', 'iconset', str(output / 'gopad-dev.icns'), '-o', str(iconset)],
                           check=True, capture_output=True)
            self.assertTrue((iconset / 'icon_512x512@2x.png').is_file())
            (output / 'keep.txt').write_text('unrelated file')
            subprocess.run(command + ['--force'], check=True, capture_output=True, text=True)
            self.assertEqual((output / 'keep.txt').read_text(), 'unrelated file')
            with self.assertRaisesRegex(icon.IconError, 'already a generated Dev PNG'):
                icon.read_source(output / 'gopad-dev.png')
            generated = output / 'gopad-dev-32.png'
            generated.write_bytes(b'manual edit')
            with self.assertRaisesRegex(icon.IconError, 'changed or replaced'):
                icon.owned_outputs(output, True)
            self.assertEqual(generated.read_bytes(), b'manual edit')
        self.assertEqual((ROOT / 'assets/gopad.png').read_bytes(), original)
