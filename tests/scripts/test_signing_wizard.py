import base64
from contextlib import redirect_stderr, redirect_stdout
import hashlib
import io
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'scripts'))
import signing_upload


@unittest.skipUnless(shutil.which('openssl'), 'OpenSSL is required for signing fixtures')
class SigningWizardTests(unittest.TestCase):
    """Exercise terminal choices against real files without touching GitHub or keychains."""

    @classmethod
    def setUpClass(cls):
        probe = subprocess.run(['openssl', 'genpkey', '-algorithm', 'ED25519', '-outform', 'DER'],
                               capture_output=True)
        if probe.returncode:
            raise unittest.SkipTest('OpenSSL with Ed25519 support is required for signing fixtures')
        temporary = tempfile.TemporaryDirectory(prefix='gopad-wizard-fixtures-')
        cls.addClassCleanup(temporary.cleanup)
        fixtures = Path(temporary.name)
        private = fixtures / 'certificate.key'
        certificate = fixtures / 'certificate.pem'
        archive = fixtures / 'certificate.p12'
        cls.password = 'ephemeral wizard fixture password'
        cls.openssl('req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1',
                    '-subj', '/CN=Gopad Wizard Test', '-addext', 'extendedKeyUsage=codeSigning',
                    '-keyout', str(private), '-out', str(certificate))
        cls.openssl('pkcs12', '-export', '-inkey', str(private), '-in', str(certificate),
                    '-out', str(archive), '-passout', 'stdin', '-keypbe', 'PBE-SHA1-3DES',
                    '-certpbe', 'PBE-SHA1-3DES', '-macalg', 'sha1',
                    input=cls.password.encode() + b'\n')
        cls.p12_data = archive.read_bytes()
        der = cls.openssl('x509', '-in', str(certificate), '-outform', 'DER')
        cls.digest = hashlib.sha256(der).hexdigest()
        cls.identity = hashlib.sha1(der).hexdigest().upper()
        public = cls.openssl('pkey', '-inform', 'DER', '-pubout', '-outform', 'DER', input=probe.stdout)
        cls.private_key = base64.b64encode(probe.stdout[-32:] + public[-32:])
        cls.public_key = base64.b64encode(public[-32:]).decode()

    @classmethod
    def openssl(cls, *args, input=None):
        result = subprocess.run(['openssl', *args], input=input, capture_output=True)
        if result.returncode:
            raise AssertionError('Could not generate ephemeral signing wizard fixture')
        return result.stdout

    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix='gopad-wizard-test-')
        self.addCleanup(temporary.cleanup)
        self.temporary = Path(temporary.name).resolve()
        self.repo = self.temporary / 'repo'
        self.config = self.repo / 'config'
        self.config.mkdir(parents=True)
        materials = self.temporary / 'materials'
        materials.mkdir(mode=0o700)
        self.p12 = materials / 'certificate.p12'
        self.key = materials / 'mygo-update.key'
        self.p12.write_bytes(self.p12_data)
        self.key.write_bytes(self.private_key + b'\n')
        self.p12.chmod(0o600)
        self.key.chmod(0o600)
        self.updates_path = self.config / 'updates.json'
        self.pin_path = self.config / 'macos-signing.json'
        self.updates = {'publicKey': '', 'github': 'fixture/gopad', 'tagPrefix': 'v', 'deltas': 0}
        self.updates_path.write_text(json.dumps(self.updates) + '\n')
        self.before_updates = self.updates_path.read_bytes()
        self.existing_secrets = set()
        self.interrupt_secret = ''
        self.commands = []
        self.uploads = []
        self.prompts = []
        self.output = io.StringIO()
        self.errors = io.StringIO()
        self.candidates = [dict(identity=self.identity, label='Existing Code Signing Identity',
                                keychain='/fixture/login.keychain-db')]
        patches = (
            mock.patch.object(signing_upload, 'ROOT', self.repo),
            mock.patch.object(signing_upload.sys, 'platform', 'darwin'),
            mock.patch.object(signing_upload.sys.stdin, 'isatty', return_value=True),
            mock.patch.object(signing_upload, 'command', side_effect=self.fake_command),
            mock.patch.object(signing_upload.getpass, 'getpass', return_value=self.password),
            mock.patch.object(signing_upload, 'inspect_p12', return_value=self.digest),
            mock.patch.object(signing_upload, 'certificate_in_keychain', return_value=self.digest),
            mock.patch.object(signing_upload.signing_keychain, 'list_identities', return_value=[]),
            mock.patch.object(signing_upload.signing_keychain, 'export_identity',
                              return_value=(self.p12_data, self.password)),
        )
        for patcher in patches:
            patched = patcher.start()
            self.addCleanup(patcher.stop)
            if patcher.attribute == 'inspect_p12':
                self.inspect = patched
            elif patcher.attribute == 'export_identity':
                self.export = patched
            elif patcher.attribute == 'list_identities':
                self.list_identities = patched

    def fake_command(self, *args, input=None):
        args = tuple(str(arg) for arg in args)
        self.commands.append((args, input))
        if args[0] == 'git':
            self.assertEqual(args[1:], ('remote', 'get-url', 'origin'))
            return b'https://github.com/fixture/gopad.git\n'
        if args[0] == 'openssl':
            result = subprocess.run(args, input=input, capture_output=True)
            if result.returncode:
                raise signing_upload.BuildError('Ephemeral OpenSSL verification failed')
            return result.stdout
        if args[:3] == ('gh', 'auth', 'status'):
            self.assertEqual(args[3:], ('--hostname', 'github.com'))
            return b''
        if args[:3] == ('gh', 'repo', 'view'):
            self.assertEqual(args[3], 'github.com/fixture/gopad')
            return b'fixture/gopad\n'
        if args[:3] == ('gh', 'secret', 'list'):
            self.assertEqual(args[args.index('--repo') + 1], 'github.com/fixture/gopad')
            return '\n'.join(sorted(self.existing_secrets)).encode()
        if args[:3] == ('gh', 'secret', 'set'):
            self.assertEqual(args[args.index('--repo') + 1], 'github.com/fixture/gopad')
            self.assertIsInstance(input, bytes)
            if args[3] == self.interrupt_secret:
                raise KeyboardInterrupt()
            self.uploads.append((args[3], input))
            self.existing_secrets.add(args[3])
            return b''
        self.fail(f'Unexpected tool use in wizard fixture: {args[0]}')

    def main(self, answers, argv=None):
        answers = iter(answers)

        def answer(prompt):
            self.prompts.append((prompt, self.output.getvalue(), list(self.commands)))
            try:
                value = next(answers)
            except StopIteration:
                self.fail(f'Wizard asked an unexpected extra question: {prompt}')
            if isinstance(value, BaseException):
                raise value
            return str(value)

        with mock.patch('builtins.input', side_effect=answer), redirect_stdout(self.output), \
                redirect_stderr(self.errors):
            return signing_upload.main([] if argv is None else argv)

    def assert_no_writes(self, expected_updates=None, expected_pin=None):
        self.assertEqual(self.uploads, [])
        self.assertEqual(self.updates_path.read_bytes(), expected_updates or self.before_updates)
        if expected_pin is None:
            self.assertFalse(self.pin_path.exists())
        else:
            self.assertEqual(self.pin_path.read_bytes(), expected_pin)

    def assert_no_secrets_printed(self):
        output = self.output.getvalue() + self.errors.getvalue()
        for secret in (self.password, self.private_key.decode(), base64.b64encode(self.p12_data).decode()):
            self.assertNotIn(secret, output)

    def test_no_arguments_show_context_before_prompt_then_upload_three_stdin_secrets(self):
        self.assertEqual(self.main(['p', self.p12, self.key, 'y']), 'fixture/gopad')
        self.assertTrue(self.prompts)
        first_prompt_output = self.prompts[0][1]
        self.assertIn('fixture/gopad', first_prompt_output)
        for name in signing_upload.SECRET_NAMES:
            self.assertIn(name, first_prompt_output)
        self.assertTrue(any(args[:3] == ('gh', 'secret', 'list')
                            for args, _ in self.prompts[0][2]))
        self.assertEqual(self.uploads, list(zip(signing_upload.SECRET_NAMES,
                            (base64.b64encode(self.p12_data), self.password.encode(), self.private_key))))
        self.assertEqual(json.loads(self.pin_path.read_text())['certificateSHA256'], self.digest)
        self.assertEqual(json.loads(self.updates_path.read_text())['publicKey'], self.public_key)
        self.inspect.assert_called_once_with(self.p12_data, self.password)
        self.export.assert_not_called()
        self.assert_no_secrets_printed()

    def test_provided_p12_only_prompts_for_missing_updater_key_and_confirmation(self):
        self.main([self.key, 'y'], ['--p12', str(self.p12)])
        self.assertEqual(len(self.prompts), 2)
        self.assertEqual(len(self.uploads), 3)
        self.export.assert_not_called()

    def test_provided_updater_key_only_prompts_for_source_p12_and_confirmation(self):
        self.main(['p', self.p12, 'y'], ['--updater-key', str(self.key)])
        self.assertEqual(len(self.prompts), 3)
        self.assertEqual(len(self.uploads), 3)
        self.export.assert_not_called()

    def test_local_identity_requires_selection_before_export(self):
        self.list_identities.return_value = self.candidates
        self.main(['1', self.key, 'y'])
        self.export.assert_called_once_with(self.identity, '/fixture/login.keychain-db',
                                            signing_upload.command)
        self.assertEqual(len(self.uploads), 3)
        self.assert_no_secrets_printed()

    def test_local_identity_pin_mismatch_is_rejected_before_export(self):
        self.list_identities.return_value = self.candidates
        self.pin_path.write_text(json.dumps({'schema': 1, 'certificateSHA256': '0' * 64}) + '\n')
        before_pin = self.pin_path.read_bytes()
        with self.assertRaisesRegex(signing_upload.BuildError, 'rotation|differs|match'):
            self.main(['1', self.key, 'y'])
        self.export.assert_not_called()
        self.assert_no_writes(expected_pin=before_pin)

    def test_cancel_before_selection_never_exports_local_identity(self):
        self.list_identities.return_value = self.candidates
        self.main(['q'])
        self.export.assert_not_called()
        self.inspect.assert_not_called()
        self.assert_no_writes()

    def test_eof_and_ctrl_c_cancel_without_configuration_or_upload(self):
        for exception in (EOFError(), KeyboardInterrupt()):
            with self.subTest(interruption=type(exception).__name__):
                self.main([exception])
                self.export.assert_not_called()
                self.assert_no_writes()

    def test_cancel_at_private_material_path_leaves_no_writes(self):
        self.main(['p', 'q'])
        self.inspect.assert_not_called()
        self.assert_no_writes()

    def test_declining_final_confirmation_does_not_save_public_configuration(self):
        self.main(['p', self.p12, self.key, 'n'])
        self.inspect.assert_called_once()
        self.assert_no_writes()
        self.assert_no_secrets_printed()

    def test_empty_final_confirmation_defaults_to_no_upload(self):
        self.main(['p', self.p12, self.key, ''])
        self.assert_no_writes()

    def test_interrupting_final_confirmation_leaves_configuration_and_github_untouched(self):
        self.main(['p', self.p12, self.key, KeyboardInterrupt()])
        self.inspect.assert_called_once()
        self.assert_no_writes()
        self.assert_no_secrets_printed()

    def test_interrupting_upload_reports_completed_secret_and_retains_public_identity(self):
        self.interrupt_secret = signing_upload.SECRET_NAMES[1]
        with self.assertRaises(signing_upload.BuildError) as error:
            self.main(['p', self.p12, self.key, 'y'])
        self.assertIn(signing_upload.SECRET_NAMES[0], str(error.exception))
        self.assertIn(signing_upload.SECRET_NAMES[1], str(error.exception))
        self.assertEqual(self.uploads, [(signing_upload.SECRET_NAMES[0],
                                        base64.b64encode(self.p12_data))])
        attempted = [args[3] for args, _ in self.commands if args[:3] == ('gh', 'secret', 'set')]
        self.assertEqual(attempted, list(signing_upload.SECRET_NAMES[:2]))
        self.assertEqual(json.loads(self.pin_path.read_text())['certificateSHA256'], self.digest)
        self.assertEqual(json.loads(self.updates_path.read_text())['publicKey'], self.public_key)
        reported = self.output.getvalue() + self.errors.getvalue() + str(error.exception)
        self.assertNotIn('未上传，也未写入公开配置', reported)
        self.assert_no_secrets_printed()

    def test_dry_run_validates_without_upload_confirmation_or_writes(self):
        self.main(['p', self.p12, self.key], ['--dry-run'])
        self.assertEqual(len(self.prompts), 3)
        self.inspect.assert_called_once()
        self.assert_no_writes()
        self.assert_no_secrets_printed()

    def test_noninteractive_missing_arguments_fail_before_reading_keys_or_remote(self):
        with mock.patch.object(signing_upload.sys.stdin, 'isatty', return_value=False):
            with self.assertRaisesRegex(signing_upload.BuildError, 'terminal|interactive|--p12|终端|交互'):
                self.main([])
        self.assertEqual(self.prompts, [])
        self.assertEqual(self.commands, [])
        self.list_identities.assert_not_called()
        self.assert_no_writes()

    def test_explicit_parameters_preserve_existing_upload_api(self):
        with mock.patch.object(signing_upload, 'upload', return_value='fixture/gopad') as upload:
            self.assertEqual(signing_upload.main(['--p12', str(self.p12), '--updater-key', str(self.key),
                                                 '--repo', 'fixture/gopad', '--dry-run']),
                             'fixture/gopad')
        upload.assert_called_once_with(str(self.p12), str(self.key), 'fixture/gopad', True)
        self.assertEqual(self.commands, [])
        self.export.assert_not_called()
        self.assert_no_writes()


if __name__ == '__main__':
    unittest.main()
