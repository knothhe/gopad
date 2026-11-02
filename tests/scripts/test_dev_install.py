import json
from pathlib import Path
import plistlib
import shutil
import signal
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
import macos


class DevInstallTests(unittest.TestCase):
    def test_process_matching_uses_exact_bundle_path(self):
        dest = Path('/Applications/gopad Dev.app')
        processes = '\n'.join([
            '  12 /Applications/gopad Dev.app/Contents/MacOS/gopad Dev',
            '  13 /Applications/gopad.app/Contents/MacOS/gopad',
            '  14 /tmp/gopad Dev.app/Contents/MacOS/gopad Dev',
            '  15 /Applications/gopad Dev.app.backup/Contents/MacOS/gopad Dev',
            '  16 /usr/bin/python3',
        ])
        with mock.patch.object(macos, 'run', return_value=processes):
            self.assertEqual(macos.dev_processes(dest), [12])

    def test_stop_waits_for_exit(self):
        with mock.patch.object(macos, 'dev_processes', side_effect=[[12], [12], []]), \
                mock.patch.object(macos.os, 'kill') as kill, \
                mock.patch.object(macos.time, 'sleep') as sleep:
            self.assertTrue(macos.stop_dev(Path('/tmp/gopad Dev.app')))
            kill.assert_called_once_with(12, signal.SIGTERM)
            sleep.assert_called_once_with(0.1)

    def test_stop_timeout_preserves_installation(self):
        with mock.patch.object(macos, 'dev_processes', return_value=[12]), \
                mock.patch.object(macos.os, 'kill'), \
                mock.patch.object(macos.time, 'monotonic', side_effect=[0, 11]):
            with self.assertRaisesRegex(macos.BuildError, 'left unchanged'):
                macos.stop_dev(Path('/tmp/gopad Dev.app'))

    def test_install_lifecycle(self):
        for running, verification_fails, replacement_fails in [
                (True, False, False), (False, False, False),
                (True, True, False), (True, False, True)]:
            with self.subTest(running=running, verification_fails=verification_fails,
                              replacement_fails=replacement_fails), tempfile.TemporaryDirectory() as temp:
                root = Path(temp).resolve()
                dest = root / 'installed/gopad Dev.app'
                contents = dest / 'Contents'
                (contents / 'Resources').mkdir(parents=True)
                (contents / 'Info.plist').write_bytes(plistlib.dumps({
                    'CFBundleIdentifier': 'dev.knothhe.gopad.dev'}))
                (contents / 'Resources/build-info.json').write_text(json.dumps({'channel': 'dev'}))
                (dest / 'version').write_text('old')
                source = root / 'build/gopad Dev.app'
                source.mkdir(parents=True)
                (source / 'version').write_text('new')
                events = []

                def run(*args, **kwargs):
                    events.append(str(args[0]))
                    if args[0] == 'ditto':
                        shutil.copytree(args[1], args[2])
                    if args[0] == 'open':
                        self.assertEqual(args[1], dest)
                        self.assertEqual((dest / 'version').read_text(),
                                         'old' if replacement_fails else 'new')
                    return ''

                def verify(*args):
                    events.append('verify')
                    if verification_fails:
                        raise macos.BuildError('verification failed')

                def stop(target):
                    self.assertEqual(target, dest)
                    self.assertEqual(events, ['ditto', 'verify'])
                    events.append('stop')
                    return running

                original_rename = Path.rename

                def rename(path, target):
                    if replacement_fails and path.name == dest.name and path != dest:
                        raise OSError('replacement failed')
                    return original_rename(path, target)

                with mock.patch.object(macos, 'build', return_value=(source.parent, {}, '', '')), \
                        mock.patch.object(macos, 'run', side_effect=run), \
                        mock.patch.object(macos, 'verify_bundle', side_effect=verify), \
                        mock.patch.object(macos, 'stop_dev', side_effect=stop) as stopper, \
                        mock.patch.object(Path, 'rename', rename):
                    if verification_fails or replacement_fails:
                        with self.assertRaises((macos.BuildError, OSError)):
                            macos.install(dest)
                    else:
                        macos.install(dest)
                if verification_fails:
                    stopper.assert_not_called()
                self.assertEqual(events.count('open'), int(running and not verification_fails))
                self.assertEqual((dest / 'version').read_text(),
                                 'old' if verification_fails or replacement_fails else 'new')
