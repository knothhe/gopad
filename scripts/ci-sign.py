#!/usr/bin/env python3
"""Import release signing material for exactly one build; restore keychain state on every exit."""
import base64
import json
import os
from pathlib import Path
import re
import secrets
import shlex
import shutil
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
from macos import ROOT, BuildError, read_pin, require


def secret_run(*args):
    result = subprocess.run([str(a) for a in args], text=True, capture_output=True, cwd=ROOT)
    if result.returncode:
        raise BuildError(f'{Path(str(args[0])).name} signing setup failed (exit {result.returncode}); secret arguments and output omitted.')
    return result.stdout.strip()


def main():
    require(sys.platform == 'darwin' and os.environ.get('GITHUB_ACTIONS') == 'true', 'This wrapper is only for the trusted macOS release CI job.')
    read_pin(ROOT / 'config/macos-signing.json')
    for name in ['MACOS_SIGNING_CERTIFICATE', 'MACOS_SIGNING_CERTIFICATE_PASSWORD', 'MYGO_UPDATER_PRIVATE_KEY']:
        require(bool(os.environ.get(name)), f'Missing CI secret {name}; release signing cannot be skipped.')
    original = shlex.split(secret_run('security', 'list-keychains', '-d', 'user'))
    root = Path(tempfile.mkdtemp(prefix='gopad-sign-', dir=os.environ.get('RUNNER_TEMP')))
    root.chmod(0o700)
    keychain = root / 'signing.keychain-db'
    created = False
    cleanup_failures = []
    try:
        p12 = root / 'certificate.p12'
        p12.write_bytes(base64.b64decode(os.environ['MACOS_SIGNING_CERTIFICATE'], validate=True))
        p12.chmod(0o600)
        password = secrets.token_urlsafe(32)
        secret_run('security', 'create-keychain', '-p', password, keychain)
        created = True
        secret_run('security', 'set-keychain-settings', '-lut', '7200', keychain)
        secret_run('security', 'unlock-keychain', '-p', password, keychain)
        secret_run('security', 'import', p12, '-k', keychain, '-f', 'pkcs12', '-P', os.environ['MACOS_SIGNING_CERTIFICATE_PASSWORD'], '-T', '/usr/bin/codesign')
        p12.unlink()
        secret_run('security', 'set-key-partition-list', '-S', 'apple-tool:,apple:,codesign:', '-s', '-k', password, keychain)
        identities = set(re.findall(r'\)\s+([A-Fa-f0-9]{40})\s+"', secret_run('security', 'find-identity', '-p', 'codesigning', keychain)))
        require(len(identities) == 1, 'CI keychain must contain exactly one paired Code Signing identity.')
        secret_run('security', 'list-keychains', '-d', 'user', '-s', keychain, *original)
        env = dict(os.environ)
        env['GOPAD_SIGNING_IDENTITY'] = next(iter(identities)).upper()
        env['GOPAD_SIGNING_KEYCHAIN'] = str(keychain)
        for name in ['MACOS_SIGNING_CERTIFICATE', 'MACOS_SIGNING_CERTIFICATE_PASSWORD']:
            env.pop(name, None)
        # Optional Developer ID notarization: all three are required together.
        notary = [os.environ.get(key, '') for key in ['APPLE_ID', 'APPLE_TEAM_ID', 'APPLE_APP_PASSWORD']]
        if any(notary):
            require(all(notary), 'Configure all three Apple notarization secrets, or none for fixed-certificate signing only.')
            secret_run('xcrun', 'notarytool', 'store-credentials', 'gopad-ci', '--keychain', keychain,
                       '--apple-id', notary[0], '--team-id', notary[1], '--password', notary[2])
            env['GOPAD_NOTARY_PROFILE'] = 'gopad-ci'
        for name in ['APPLE_ID', 'APPLE_TEAM_ID', 'APPLE_APP_PASSWORD']:
            env.pop(name, None)
        result = subprocess.run([sys.executable, str(ROOT / 'scripts/macos.py'), 'package', '--channel', 'release'], cwd=ROOT, env=env)
        require(result.returncode == 0, 'Release package build failed.')
    finally:
        try:
            secret_run('security', 'list-keychains', '-d', 'user', '-s', *original)
        except BuildError as error:
            cleanup_failures.append(str(error))
        if created or keychain.exists():
            try:
                secret_run('security', 'delete-keychain', keychain)
            except BuildError as error:
                cleanup_failures.append(str(error))
        try:
            shutil.rmtree(root)
        except OSError as error:
            cleanup_failures.append(str(error))
        if cleanup_failures:
            raise BuildError('Signing cleanup incomplete: ' + '; '.join(cleanup_failures))

if __name__ == '__main__':
    try:
        main()
    except (BuildError, OSError, ValueError) as error:
        print(f'ERROR: {error}', file=sys.stderr)
        sys.exit(1)
