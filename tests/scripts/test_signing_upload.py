import base64
from contextlib import redirect_stderr, redirect_stdout
import hashlib
import io
import json
import os
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
class SigningUploadTests(unittest.TestCase):
    """Real signing material and OpenSSL; isolated stand-ins for GitHub and keychains."""

    @classmethod
    def setUpClass(cls):
        cls.real_run = staticmethod(subprocess.run)
        # Signing upload needs Ed25519, while ordinary builds also support the
        # older LibreSSL shipped with macOS. Do not break their unrelated checks.
        probe = cls.real_run(['openssl', 'genpkey', '-algorithm', 'ED25519', '-outform', 'DER'],
                             capture_output=True)
        if probe.returncode:
            raise unittest.SkipTest('OpenSSL with Ed25519 support is required for signing upload tests')
        cls.fixture_temp = tempfile.TemporaryDirectory(prefix='gopad-signing-fixtures-')
        cls.addClassCleanup(cls.fixture_temp.cleanup)
        fixtures = Path(cls.fixture_temp.name)
        cls.password = 'fixture P12 password'
        private = fixtures / 'certificate.key'
        certificate = fixtures / 'certificate.pem'
        p12 = fixtures / 'certificate.p12'
        cls.openssl('req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1',
                    '-subj', '/CN=Gopad Signing Test', '-addext', 'extendedKeyUsage=codeSigning',
                    '-keyout', str(private), '-out', str(certificate))
        cls.openssl('pkcs12', '-export', '-inkey', str(private), '-in', str(certificate),
                    '-out', str(p12), '-passout', 'stdin', '-keypbe', 'PBE-SHA1-3DES',
                    '-certpbe', 'PBE-SHA1-3DES', '-macalg', 'sha1',
                    input=cls.password.encode() + b'\n')
        cls.p12_data = p12.read_bytes()
        cls.certificate_pem = certificate.read_bytes()
        der = cls.openssl('x509', '-in', str(certificate), '-outform', 'DER')
        cls.identity = hashlib.sha1(der).hexdigest().upper()
        cls.digest = hashlib.sha256(der).hexdigest()
        ed_private = probe.stdout
        ed_public = cls.openssl('pkey', '-inform', 'DER', '-pubout', '-outform', 'DER',
                                input=ed_private)
        cls.private_key = base64.b64encode(ed_private[-32:] + ed_public[-32:])
        cls.public_key = base64.b64encode(ed_public[-32:]).decode()

    @classmethod
    def openssl(cls, *args, input=None):
        result = cls.real_run(['openssl', *args], input=input, capture_output=True)
        if result.returncode:
            raise AssertionError(result.stderr.decode())
        return result.stdout

    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix='gopad-signing-test-')
        self.addCleanup(temporary.cleanup)
        self.temporary = Path(temporary.name).resolve()
        self.repo = self.temporary / 'repo'
        self.config = self.repo / 'config'
        self.config.mkdir(parents=True)
        materials = self.temporary / 'materials'
        materials.mkdir(mode=0o700)
        self.p12 = materials / 'certificate.p12'
        self.p12.write_bytes(self.p12_data)
        self.key = materials / 'mygo-update.key'
        self.key.write_bytes(self.private_key + b'\n')
        for path in (self.p12, self.key):
            path.chmod(0o600)
        self.updates_path = self.config / 'updates.json'
        self.pin_path = self.config / 'macos-signing.json'
        self.updates = {'publicKey': '', 'github': 'fixture/gopad', 'tagPrefix': 'v', 'deltas': 0}
        self.write_updates()
        self.origin = 'https://github.com/fixture/gopad.git'
        self.canonical_repo = 'fixture/gopad'
        self.existing_secrets = set()
        self.security_calls = []
        self.gh_calls = []
        self.uploads = []
        self.created_keychains = []
        self.deleted_keychains = []
        self.signing_temp_dirs = set()
        self.native_identities = [self.identity]
        self.fail_secret = ''
        self.fail_restore = False
        self.fail_native_import = False
        self.output = io.StringIO()
        self.errors = io.StringIO()
        for patcher in (
            mock.patch.object(signing_upload, 'ROOT', self.repo),
            mock.patch.object(signing_upload.sys, 'platform', 'darwin'),
            mock.patch.object(signing_upload.sys.stdin, 'isatty', return_value=True),
            mock.patch.object(signing_upload.getpass, 'getpass', return_value=self.password),
            mock.patch.object(signing_upload.subprocess, 'run', side_effect=self.fake_run),
        ):
            patcher.start()
            self.addCleanup(patcher.stop)

    def write_updates(self):
        self.updates_path.write_text(json.dumps(self.updates) + '\n')

    def write_pin(self, digest=None):
        self.pin_path.write_text(json.dumps({'schema': 1, 'certificateSHA256': digest or self.digest}) + '\n')

    def result(self, args, stdout=b'', status=0):
        return subprocess.CompletedProcess(args, status, stdout,
                                           b'PRIVATE TOOL DIAGNOSTIC' if status else b'')

    def fake_run(self, args, **kwargs):
        args = [str(arg) for arg in args]
        self.assertFalse(kwargs.get('shell', False))
        executable = Path(args[0]).name
        if executable == 'openssl':
            if args[1] == 'pkcs12' and '-in' in args:
                self.signing_temp_dirs.add(Path(args[args.index('-in') + 1]).parent)
            return self.real_run(args, **kwargs)
        if executable == 'git':
            self.assertEqual(args[1:], ['remote', 'get-url', 'origin'])
            return self.result(args, self.origin.encode() + b'\n')
        if executable == 'security':
            self.security_calls.append(args)
            action = args[1]
            if action == 'list-keychains':
                if '-s' in args:
                    self.assertEqual(args[2:], ['-d', 'user', '-s', '/fixture/login.keychain-db'])
                    return self.result(args, status=int(self.fail_restore))
                return self.result(args, b'    "/fixture/login.keychain-db"\n')
            if action == 'create-keychain':
                keychain = Path(args[-1])
                self.assertEqual(keychain.parent.stat().st_mode & 0o777, 0o700)
                keychain.touch(mode=0o600)
                self.created_keychains.append(keychain)
            elif action == 'unlock-keychain':
                self.assertIn(Path(args[-1]), self.created_keychains)
            elif action == 'import':
                p12 = Path(args[2])
                self.assertEqual(p12.stat().st_mode & 0o777, 0o600)
                self.assertIn(Path(args[args.index('-k') + 1]), self.created_keychains)
                password = args[args.index('-P') + 1]
                result = self.real_run(['openssl', 'pkcs12', '-in', str(p12), '-noout',
                                        '-passin', 'stdin'], input=password.encode() + b'\n',
                                       capture_output=True)
                return self.result(args, status=result.returncode or int(self.fail_native_import))
            elif action == 'find-identity':
                self.assertNotIn('-v', args)
                identities = b'\n'.join(f'  {n}) {identity} "Fixture"'.encode()
                                        for n, identity in enumerate(self.native_identities, 1))
                return self.result(args, identities)
            elif action == 'find-certificate':
                return self.result(args, self.certificate_pem)
            elif action == 'delete-keychain':
                keychain = Path(args[-1])
                self.deleted_keychains.append(keychain)
                keychain.unlink(missing_ok=True)
            else:
                self.fail(f'Unexpected security operation: {action}')
            return self.result(args)
        if executable == 'gh':
            self.gh_calls.append(args)
            if args[1:3] == ['auth', 'status']:
                self.assertEqual(args[3:], ['--hostname', 'github.com'])
                return self.result(args)
            if args[1:3] == ['repo', 'view']:
                self.assertEqual(args[3:], ['github.com/fixture/gopad', '--json', 'nameWithOwner',
                                            '--jq', '.nameWithOwner'])
                return self.result(args, self.canonical_repo.encode() + b'\n')
            if args[1:3] == ['secret', 'list']:
                self.assertEqual(args[args.index('--repo') + 1], 'github.com/fixture/gopad')
                return self.result(args, '\n'.join(sorted(self.existing_secrets)).encode())
            if args[1:3] == ['secret', 'set']:
                name = args[3]
                self.uploads.append((args, kwargs.get('input')))
                if name == self.fail_secret:
                    return self.result(args, status=1)
                self.existing_secrets.add(name)
                return self.result(args)
            self.fail(f'Unexpected gh operation: {args[1:3]}')
        self.fail(f'Unexpected subprocess: {executable}')

    def upload(self, **kwargs):
        with redirect_stdout(self.output), redirect_stderr(self.errors):
            return signing_upload.upload(str(self.p12), str(self.key), **kwargs)

    def assert_cleaned(self):
        self.assertEqual(self.created_keychains, self.deleted_keychains)
        for directory in self.signing_temp_dirs:
            self.assertFalse(directory.exists())

    def assert_no_secret_output(self, *messages):
        output = self.output.getvalue() + self.errors.getvalue() + '\n'.join(messages)
        for value in [self.password, self.private_key.decode(),
                      base64.b64encode(self.p12_data).decode(), 'PRIVATE TOOL DIAGNOSTIC']:
            self.assertNotIn(value, output)

    def test_upload_initializes_public_config_and_sends_only_stdin_secrets(self):
        self.assertEqual(self.upload(), 'fixture/gopad')
        self.assertEqual(json.loads(self.pin_path.read_text()),
                         {'schema': 1, 'certificateSHA256': self.digest})
        self.assertEqual(json.loads(self.updates_path.read_text()),
                         dict(self.updates, publicKey=self.public_key))
        self.assertEqual([args[3] for args, _ in self.uploads], list(signing_upload.SECRET_NAMES))
        self.assertEqual([value for _, value in self.uploads],
                         [base64.b64encode(self.p12_data), self.password.encode(), self.private_key])
        for args, _ in self.uploads:
            self.assertEqual(args[4:], ['--repo', 'github.com/fixture/gopad'])
            self.assertNotIn('--body', args)
            self.assertNotIn(self.password, args)
            self.assertNotIn(self.private_key.decode(), args)
        self.assert_cleaned()
        self.assert_no_secret_output()

    def test_enterprise_host_environment_cannot_redirect_github_secrets(self):
        with mock.patch.dict(os.environ, {'GH_HOST': 'enterprise.example.invalid',
                                         'GH_REPO': 'enterprise.example.invalid/other/repo'}):
            self.upload()
        self.assertEqual(len(self.uploads), 3)
        for args in self.gh_calls:
            if args[1:3] == ['auth', 'status']:
                self.assertEqual(args[args.index('--hostname') + 1], 'github.com')
            elif args[1:3] == ['repo', 'view']:
                self.assertEqual(args[3], 'github.com/fixture/gopad')
            else:
                self.assertEqual(args[args.index('--repo') + 1], 'github.com/fixture/gopad')
        self.assert_no_secret_output()

    def test_dry_run_validates_without_config_writes_or_uploads(self):
        before = self.updates_path.read_bytes()
        self.upload(dry_run=True)
        self.assertTrue(self.created_keychains)
        self.assertTrue(self.gh_calls)
        self.assertEqual(self.uploads, [])
        self.assertFalse(self.pin_path.exists())
        self.assertEqual(self.updates_path.read_bytes(), before)
        self.assert_cleaned()
        self.assert_no_secret_output()

    def test_wrong_p12_password_cleans_up_before_upload(self):
        with mock.patch.object(signing_upload.getpass, 'getpass', return_value='wrong password'):
            with self.assertRaisesRegex(signing_upload.BuildError, 'openssl pkcs12 failed') as error:
                self.upload()
        self.assertEqual(self.uploads, [])
        self.assertFalse(self.pin_path.exists())
        self.assert_cleaned()
        self.assert_no_secret_output(str(error.exception))

    def test_native_import_failure_cleans_up_before_upload(self):
        self.fail_native_import = True
        with self.assertRaisesRegex(signing_upload.BuildError, 'security import failed') as error:
            self.upload()
        self.assertTrue(self.created_keychains)
        self.assertEqual(self.uploads, [])
        self.assert_cleaned()
        self.assert_no_secret_output(str(error.exception))

    def test_extra_private_key_is_rejected_before_creating_keychain(self):
        original_command = signing_upload.command

        def extra_private_key(*args, **kwargs):
            output = original_command(*args, **kwargs)
            if args[:2] == ('openssl', 'pkcs12') and '-nocerts' in args:
                return output + output
            return output

        with mock.patch.object(signing_upload, 'command', side_effect=extra_private_key):
            with self.assertRaisesRegex(signing_upload.BuildError, 'exactly one private key'):
                self.upload()
        self.assertEqual(self.created_keychains, [])
        self.assertEqual(self.uploads, [])
        self.assert_cleaned()

    def test_certificate_pin_mismatch_is_not_rotated(self):
        self.write_pin('0' * 64)
        before = self.pin_path.read_bytes()
        with self.assertRaisesRegex(signing_upload.BuildError, 'refusing certificate rotation'):
            self.upload()
        self.assertEqual(self.pin_path.read_bytes(), before)
        self.assertEqual(self.uploads, [])
        self.assert_cleaned()

    def test_update_public_key_mismatch_is_not_rotated(self):
        self.updates['publicKey'] = base64.b64encode(b'\0' * 32).decode()
        self.write_updates()
        before = self.updates_path.read_bytes()
        with self.assertRaisesRegex(signing_upload.BuildError, 'refusing key rotation'):
            self.upload()
        self.assertEqual(self.updates_path.read_bytes(), before)
        self.assertEqual(self.created_keychains, [])
        self.assertEqual(self.uploads, [])

    def test_corrupt_seed_is_rejected_even_with_matching_public_half(self):
        key = bytearray(base64.b64decode(self.private_key))
        key[0] ^= 1
        self.key.write_bytes(base64.b64encode(key))
        self.updates['publicKey'] = self.public_key
        self.write_updates()
        with self.assertRaisesRegex(signing_upload.BuildError, 'inconsistent seed and public key'):
            self.upload()
        self.assertEqual(self.uploads, [])
        self.assertEqual(self.created_keychains, [])

    def test_target_must_match_origin_and_update_repository(self):
        with self.assertRaisesRegex(signing_upload.BuildError, 'must match origin'):
            self.upload(repo='another/project')
        self.origin = 'https://github.com/another/project.git'
        with self.assertRaisesRegex(signing_upload.BuildError, 'must match origin'):
            self.upload()
        self.assertEqual(self.gh_calls, [])
        self.assertEqual(self.uploads, [])

    def test_github_canonical_repository_is_checked(self):
        self.canonical_repo = 'moved/project'
        with self.assertRaisesRegex(signing_upload.BuildError, 'repository identity differs'):
            self.upload()
        self.assertEqual(self.uploads, [])
        self.assertFalse(self.pin_path.exists())

    def test_unknown_remote_certificate_secrets_require_original_pin(self):
        for name in signing_upload.SECRET_NAMES[:2]:
            with self.subTest(secret=name):
                self.existing_secrets = {name}
                with self.assertRaisesRegex(signing_upload.BuildError, 'Restore the original public pin'):
                    self.upload()
                self.assertFalse(self.pin_path.exists())
                self.assertEqual(self.uploads, [])
        self.assert_cleaned()

    def test_unknown_remote_update_secret_requires_original_public_key(self):
        self.write_pin()
        self.existing_secrets = {signing_upload.SECRET_NAMES[2]}
        before = self.updates_path.read_bytes()
        with self.assertRaisesRegex(signing_upload.BuildError, 'Restore the original public key'):
            self.upload()
        self.assertEqual(self.updates_path.read_bytes(), before)
        self.assertEqual(self.uploads, [])
        self.assert_cleaned()

    def test_partial_failure_keeps_public_identity_and_supports_same_key_retry(self):
        self.fail_secret = signing_upload.SECRET_NAMES[1]
        with self.assertRaisesRegex(signing_upload.BuildError,
                                    'Successfully uploaded: MACOS_SIGNING_CERTIFICATE') as error:
            self.upload()
        self.assertEqual([args[3] for args, _ in self.uploads], list(signing_upload.SECRET_NAMES[:2]))
        self.assertEqual(json.loads(self.pin_path.read_text())['certificateSHA256'], self.digest)
        self.assertEqual(json.loads(self.updates_path.read_text())['publicKey'], self.public_key)
        self.assert_no_secret_output(str(error.exception))
        self.fail_secret = ''
        self.uploads.clear()
        self.upload()
        self.assertEqual([args[3] for args, _ in self.uploads], list(signing_upload.SECRET_NAMES))
        self.assert_cleaned()
        self.assert_no_secret_output()

    def test_search_list_restore_failure_still_deletes_keychain_and_blocks_upload(self):
        self.fail_restore = True
        with self.assertRaisesRegex(signing_upload.BuildError, 'Signing cleanup incomplete') as error:
            self.upload()
        self.assert_cleaned()
        self.assertEqual(self.uploads, [])
        self.assertFalse(self.pin_path.exists())
        self.assert_no_secret_output(str(error.exception))

    def test_multiple_native_identities_are_rejected(self):
        self.native_identities.append('B' * 40)
        with self.assertRaisesRegex(signing_upload.BuildError, 'exactly one paired'):
            self.upload()
        self.assertEqual(self.uploads, [])
        self.assert_cleaned()

    def test_no_native_paired_identity_is_rejected(self):
        self.native_identities = []
        with self.assertRaisesRegex(signing_upload.BuildError, 'exactly one paired'):
            self.upload()
        self.assertEqual(self.uploads, [])
        self.assert_cleaned()

    def test_no_tty_fails_without_getpass_fallback(self):
        with mock.patch.object(signing_upload.sys.stdin, 'isatty', return_value=False), \
                mock.patch.object(signing_upload.getpass, 'getpass') as getpass:
            with self.assertRaisesRegex(signing_upload.BuildError, 'interactive terminal'):
                self.upload()
            getpass.assert_not_called()
        self.assertEqual(self.created_keychains, [])
        self.assertEqual(self.uploads, [])

    def test_private_inputs_must_be_real_nonempty_files_outside_repo(self):
        cases = [('missing', self.temporary / 'missing.p12'), ('directory', self.temporary),
                 ('inside repository', self.repo / 'certificate.p12'),
                 ('empty', self.temporary / 'empty.p12')]
        (self.repo / 'certificate.p12').write_bytes(self.p12_data)
        (self.temporary / 'empty.p12').touch()
        for label, path in cases:
            with self.subTest(label=label):
                with self.assertRaises(signing_upload.BuildError):
                    signing_upload.upload(str(path), str(self.key))
        with self.assertRaisesRegex(signing_upload.BuildError, 'path is required'):
            signing_upload.upload('', str(self.key))
        self.assertEqual(self.created_keychains, [])
        self.assertEqual(self.uploads, [])

    def test_symlinked_public_config_is_rejected(self):
        external = self.temporary / 'updates.json'
        self.updates_path.rename(external)
        self.updates_path.symlink_to(external)
        with self.assertRaisesRegex(signing_upload.BuildError, 'symbolic-link'):
            self.upload()
        self.assertEqual(self.created_keychains, [])
        self.assertEqual(self.uploads, [])

    def test_symlinked_pin_is_rejected(self):
        external = self.temporary / 'macos-signing.json'
        external.write_text(json.dumps({'schema': 1, 'certificateSHA256': self.digest}))
        self.pin_path.symlink_to(external)
        with self.assertRaisesRegex(signing_upload.BuildError, 'symbolic-link'):
            self.upload()
        self.assertEqual(self.created_keychains, [])
        self.assertEqual(self.uploads, [])


if __name__ == '__main__':
    unittest.main()
