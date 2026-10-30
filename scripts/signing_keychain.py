"""Select an existing keychain identity and export only its certificate/key pair."""
from __future__ import annotations
import base64
import hashlib
import os
from pathlib import Path
import re
import secrets
import shlex
import shutil
import tempfile

from macos import BuildError, require


def list_identities(run):
    """Return paired Code Signing identities without choosing or exporting any."""
    configured = os.environ.get('GOPAD_SIGNING_KEYCHAIN', '').strip()
    keychains = ([configured] if configured else
                 shlex.split(run('security', 'list-keychains', '-d', 'user').decode()))
    identities = []
    seen = set()
    for keychain in keychains:
        output = run('security', 'find-identity', '-p', 'codesigning', keychain).decode()
        for identity, label in re.findall(r'\)\s+([A-Fa-f0-9]{40})\s+"([^\n"]+)"', output):
            identity = identity.upper()
            if identity not in seen:
                identities.append({'identity': identity, 'label': label, 'keychain': keychain})
                seen.add(identity)
    return identities


def _private_write(path, data):
    with path.open('xb') as stream:
        path.chmod(0o600)
        stream.write(data)


def export_identity(identity, keychain, run):
    """Repackage exactly the selected pair; never return the aggregate export."""
    identity = identity.upper()
    require(re.fullmatch('[A-F0-9]{40}', identity) is not None,
            'Choose an exact Code Signing identity SHA1 before exporting.')
    directory = Path(tempfile.mkdtemp(prefix='gopad-signing-export-'))
    try:
        directory.chmod(0o700)
        exported = directory / 'keychain.p12'
        _private_write(exported, b'')
        export_password = secrets.token_urlsafe(32)
        run('security', 'export', '-k', keychain, '-t', 'identities', '-f', 'pkcs12',
            '-P', export_password, '-o', exported)
        exported.chmod(0o600)
        arguments = ('openssl', 'pkcs12', '-in', exported, '-nodes', '-passin', 'stdin')
        try:
            pem = run(*arguments, input=export_password.encode() + b'\n')
        except BuildError:
            if not run('openssl', 'version').startswith(b'OpenSSL 3.'):
                raise
            pem = run(*arguments, '-legacy', input=export_password.encode() + b'\n')
        certificates = []
        for block in re.findall(rb'-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----', pem, re.S):
            der = base64.b64decode(b''.join(block.splitlines()[1:-1]), validate=True)
            if hashlib.sha1(der).hexdigest().upper() == identity:
                certificates.append(block)
        require(len(certificates) == 1,
                'Keychain export must contain exactly one copy of the selected certificate.')
        certificate = certificates[0]
        public_pem = run('openssl', 'x509', '-pubkey', '-noout', input=certificate)
        public_key = run('openssl', 'pkey', '-pubin', '-outform', 'DER', input=public_pem)
        matched_keys = []
        for match in re.finditer(rb'-----BEGIN ((?:[A-Z0-9]+ )*PRIVATE KEY)-----.*?-----END \1-----', pem, re.S):
            private_key = match.group(0)
            if run('openssl', 'pkey', '-pubout', '-outform', 'DER', input=private_key) == public_key:
                matched_keys.append(private_key)
        require(len(matched_keys) == 1,
                'Keychain export must contain exactly one private key matching the selected certificate.')
        certificate_path = directory / 'certificate.pem'
        private_path = directory / 'private.pem'
        archive = directory / 'selected.p12'
        _private_write(certificate_path, certificate)
        _private_write(private_path, matched_keys[0])
        _private_write(archive, b'')
        password = secrets.token_urlsafe(32)
        run('openssl', 'pkcs12', '-export', '-in', certificate_path, '-inkey', private_path,
            '-out', archive, '-passout', 'stdin', '-keypbe', 'PBE-SHA1-3DES',
            '-certpbe', 'PBE-SHA1-3DES', '-macalg', 'sha1', input=password.encode() + b'\n')
        data = archive.read_bytes()
        require(bool(data), 'Selected identity export produced an empty P12.')
        return data, password
    finally:
        try:
            shutil.rmtree(directory)
        except OSError as error:
            raise BuildError('Private keychain export cleanup failed; no secrets uploaded.') from error
