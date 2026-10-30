import base64
from contextlib import redirect_stderr, redirect_stdout
import hashlib
import io
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'scripts'))
import signing_keychain
from macos import BuildError


@unittest.skipUnless(shutil.which('openssl'), 'OpenSSL is required for export fixtures')
class KeychainExportTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.fixtures = tempfile.TemporaryDirectory(prefix='gopad-export-fixtures-')
        cls.addClassCleanup(cls.fixtures.cleanup)
        cls.pairs = []
        for number in range(2):
            directory = Path(cls.fixtures.name)
            key = directory / f'key-{number}.pem'
            certificate = directory / f'cert-{number}.pem'
            cls.openssl('req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1',
                        '-subj', f'/CN=Gopad Export Fixture {number}',
                        '-keyout', str(key), '-out', str(certificate))
            der = cls.openssl('x509', '-in', str(certificate), '-outform', 'DER')
            cls.pairs.append((hashlib.sha1(der).hexdigest().upper(),
                              certificate.read_bytes(), key.read_bytes()))

    @staticmethod
    def openssl(*args, input=None):
        result = subprocess.run(['openssl', *args], input=input, capture_output=True)
        if result.returncode:
            raise AssertionError('OpenSSL export fixture command failed')
        return result.stdout

    def setUp(self):
        self.aggregate = b'\n'.join(certificate + key for _, certificate, key in self.pairs)
        self.calls = []
        self.export_directories = []
        self.fail_export = False
        self.fail_parse = False
        self.require_legacy = False
        self.stdout = io.StringIO()
        self.stderr = io.StringIO()
        self.environment = mock.patch.dict(os.environ, {}, clear=True)
        self.environment.start()
        self.addCleanup(self.environment.stop)

    def fake_run(self, *args, input=None):
        args = tuple(str(arg) for arg in args)
        self.calls.append(args)
        if args[:2] == ('security', 'list-keychains'):
            return b'"/fixture/login.keychain-db"\n"/fixture/extra.keychain-db"\n'
        if args[:2] == ('security', 'find-identity'):
            self.assertEqual(args[2:4], ('-p', 'codesigning'))
            self.assertNotIn('-v', args)
            selected = self.pairs if args[-1] == '/fixture/login.keychain-db' else self.pairs[:1]
            return b'\n'.join(f' {number}) {identity} "Fixture {number}"'.encode()
                              for number, (identity, _, _) in enumerate(selected, 1))
        if args[:2] == ('security', 'export'):
            self.assertEqual(args[2:8], ('-k', '/fixture/login.keychain-db', '-t', 'identities', '-f', 'pkcs12'))
            exported = Path(args[args.index('-o') + 1])
            self.export_directories.append(exported.parent)
            self.assertEqual(exported.parent.stat().st_mode & 0o777, 0o700)
            self.assertEqual(exported.stat().st_mode & 0o777, 0o600)
            exported.write_bytes(b'opaque aggregate keychain export')
            if self.fail_export:
                raise BuildError('security export failed; output omitted')
            return b''
        if args[:2] == ('openssl', 'pkcs12') and '-nodes' in args:
            self.assertTrue(input)
            if self.fail_parse or (self.require_legacy and '-legacy' not in args):
                raise BuildError('openssl pkcs12 failed; output omitted')
            return self.aggregate
        if args == ('openssl', 'version'):
            return b'OpenSSL 3.6 fixture'
        if args[:3] == ('openssl', 'pkcs12', '-export'):
            for flag in ('-in', '-inkey', '-out'):
                path = Path(args[args.index(flag) + 1])
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            self.assertEqual(args[args.index('-passout') + 1], 'stdin')
        if args[0] == 'openssl':
            return self.openssl(*args[1:], input=input)
        self.fail(f'Unexpected export test command: {args[:2]}')

    def export(self, identity=None):
        with redirect_stdout(self.stdout), redirect_stderr(self.stderr):
            return signing_keychain.export_identity(identity or self.pairs[1][0],
                                                     '/fixture/login.keychain-db', self.fake_run)

    def assert_cleaned(self):
        self.assertTrue(self.export_directories)
        for directory in self.export_directories:
            self.assertFalse(directory.exists())
        self.assertEqual(self.stdout.getvalue(), '')
        self.assertEqual(self.stderr.getvalue(), '')

    def test_list_identities_keeps_order_and_deduplicates_without_exporting(self):
        identities = signing_keychain.list_identities(self.fake_run)
        self.assertEqual([item['identity'] for item in identities], [pair[0] for pair in self.pairs])
        self.assertEqual([item['keychain'] for item in identities], ['/fixture/login.keychain-db'] * 2)
        self.assertFalse(any(args[:2] == ('security', 'export') for args in self.calls))

    def test_explicit_keychain_is_the_only_source(self):
        with mock.patch.dict(os.environ, {'GOPAD_SIGNING_KEYCHAIN': '/fixture/extra.keychain-db'}):
            identities = signing_keychain.list_identities(self.fake_run)
        self.assertEqual([item['identity'] for item in identities], [self.pairs[0][0]])
        self.assertEqual(len(self.calls), 1)
        self.assertEqual(self.calls[0][-1], '/fixture/extra.keychain-db')

    def test_repackages_only_the_selected_certificate_and_matching_private_key(self):
        data, password = self.export()
        with tempfile.TemporaryDirectory() as temporary:
            archive = Path(temporary) / 'selected.p12'
            archive.write_bytes(data)
            pem = self.openssl('pkcs12', '-in', str(archive), '-nodes', '-passin', 'stdin',
                               input=password.encode() + b'\n')
        certificates = re.findall(rb'-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----', pem, re.S)
        self.assertEqual(len(certificates), 1)
        der = base64.b64decode(b''.join(certificates[0].splitlines()[1:-1]))
        self.assertEqual(hashlib.sha1(der).hexdigest().upper(), self.pairs[1][0])
        private_keys = list(re.finditer(rb'-----BEGIN ((?:[A-Z0-9]+ )*PRIVATE KEY)-----.*?-----END \1-----', pem, re.S))
        self.assertEqual(len(private_keys), 1)
        self.assertEqual(self.openssl('pkey', '-pubout', '-outform', 'DER', input=private_keys[0].group(0)),
                         self.openssl('pkey', '-pubout', '-outform', 'DER', input=self.pairs[1][2]))
        self.assert_cleaned()

    def test_missing_selected_certificate_rejects_the_export(self):
        with self.assertRaisesRegex(BuildError, 'selected certificate'):
            self.export('A' * 40)
        self.assert_cleaned()

    def test_missing_matching_private_key_rejects_the_export(self):
        self.aggregate = self.pairs[1][1] + self.pairs[0][2]
        with self.assertRaisesRegex(BuildError, 'matching the selected certificate'):
            self.export()
        self.assert_cleaned()

    def test_duplicate_matching_private_keys_are_rejected(self):
        self.aggregate += self.pairs[1][2]
        with self.assertRaisesRegex(BuildError, 'exactly one private key'):
            self.export()
        self.assert_cleaned()

    def test_export_failure_cleans_partial_material(self):
        self.fail_export = True
        with self.assertRaisesRegex(BuildError, 'security export failed'):
            self.export()
        self.assert_cleaned()

    def test_parse_failure_cleans_aggregate_material(self):
        self.fail_parse = True
        with self.assertRaisesRegex(BuildError, 'openssl pkcs12 failed'):
            self.export()
        self.assert_cleaned()

    def test_legacy_keychain_export_retries_with_openssl_legacy_provider(self):
        self.require_legacy = True
        data, password = self.export()
        self.assertTrue(data)
        self.assertTrue(password)
        parsed = [args for args in self.calls if args[:2] == ('openssl', 'pkcs12') and '-nodes' in args]
        self.assertEqual(len(parsed), 2)
        self.assertNotIn('-legacy', parsed[0])
        self.assertIn('-legacy', parsed[1])
        self.assert_cleaned()

    def test_invalid_identity_is_rejected_before_any_export(self):
        with self.assertRaisesRegex(BuildError, 'exact Code Signing identity'):
            self.export('wrong identity')
        self.assertEqual(self.calls, [])

    def test_cleanup_failure_blocks_returning_signing_material(self):
        with mock.patch.object(signing_keychain.shutil, 'rmtree', side_effect=OSError('fixture cleanup failure')):
            with self.assertRaisesRegex(BuildError, 'cleanup failed; no secrets uploaded'):
                self.export()
        for directory in self.export_directories:
            shutil.rmtree(directory)
        self.assert_cleaned()


if __name__ == '__main__':
    unittest.main()
