#!/usr/bin/env python3
"""Build, verify and install gopad bundles. No release or keychain mutation in build."""
from __future__ import annotations
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time

sys.dont_write_bytecode = True
from release import parse, ReleaseError

ROOT = Path(__file__).resolve().parents[1]
LOCAL = ROOT / '.gopad-local.json'
TARGET = 'darwin-arm64'

class BuildError(Exception):
    pass

def require(ok, message):
    if not ok:
        raise BuildError(message)

def run(*args, capture=False, env=None, cwd=ROOT, input=None):
    result = subprocess.run([str(a) for a in args], cwd=cwd, env=env, text=True, input=input,
                            stdout=subprocess.PIPE if capture else None, stderr=subprocess.PIPE if capture else None)
    if result.returncode:
        # Never print argv: CI imports pass secret values to security subprocesses.
        raise BuildError(f'{Path(str(args[0])).name} failed (exit {result.returncode})' +
                         (f': {result.stderr.strip()}' if capture else ''))
    return result.stdout.strip() if capture else ''

def version():
    value = (ROOT / 'VERSION').read_text().strip()
    parse(value)
    return value

def identity_for(channel):
    return ('gopad', 'dev.knothhe.gopad') if channel == 'release' else ('gopad Dev', 'dev.knothhe.gopad.dev')

def no_symlink(path):
    path = Path(os.path.abspath(path))
    for part in [path, *path.parents]:
        require(not part.is_symlink(), f'Refusing symbolic-link path: {part}')
    return path

def read_pin(path):
    require(path.is_file() and not path.is_symlink(), f'Missing signing pin: {path}. See docs/build-release.md.')
    value = json.loads(path.read_text())
    require(set(value) == {'schema', 'certificateSHA256'} and value['schema'] == 1 and
            re.fullmatch('[a-f0-9]{64}', value['certificateSHA256'] or ''), 'Invalid certificate pin schema.')
    return value['certificateSHA256']

def available_identities():
    keychain = os.environ.get('GOPAD_SIGNING_KEYCHAIN', '')
    args = [keychain] if keychain else []
    output = run('security', 'find-identity', '-p', 'codesigning', *args, capture=True)
    return {h.upper(): name for h, name in re.findall(r'\)\s+([0-9A-Fa-f]{40})\s+"([^"]+)"', output)}

def certificate(identity):
    """Find the exact identity's public certificate; validate EKU and expiration."""
    keychain = os.environ.get('GOPAD_SIGNING_KEYCHAIN', '')
    pem = run('security', 'find-certificate', '-a', '-p', *([keychain] if keychain else []), capture=True)
    for block in re.findall(r'-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----', pem, re.S):
        der = base64.b64decode(''.join(block.splitlines()[1:-1]))
        if hashlib.sha1(der).hexdigest().upper() != identity:
            continue
        with tempfile.TemporaryDirectory(prefix='gopad-public-cert-') as tmp:
            path = Path(tmp) / 'certificate.pem'
            path.write_text(block)
            run('openssl', 'x509', '-in', path, '-noout', '-checkend', '0', capture=True)
            description = run('openssl', 'x509', '-in', path, '-noout', '-text', capture=True)
            require('Code Signing' in description, 'Certificate has no Code Signing extended key usage.')
            dates = run('openssl', 'x509', '-in', path, '-noout', '-dates', capture=True)
        return hashlib.sha256(der).hexdigest(), dates
    raise BuildError('Signing identity has no matching public certificate.')

def signing(channel):
    if channel == 'preview':
        return '-', ''
    explicit = os.environ.get('GOPAD_SIGNING_IDENTITY', '').strip().upper()
    require(not LOCAL.is_symlink(), 'Local signing configuration cannot be a symbolic link.')
    local = json.loads(LOCAL.read_text()) if LOCAL.is_file() else {}
    identity = explicit or (local.get('identity', '') if channel == 'dev' else '')
    require(re.fullmatch('[A-F0-9]{40}', identity or '') is not None,
            'A fixed signing identity is required. Run make signing-list, then make signing-config IDENTITY=<SHA1>. '
            'For release, set GOPAD_SIGNING_IDENTITY. Explicit non-installable QA: make build CHANNEL=preview.')
    require(identity in available_identities(), 'Configured identity is unavailable; run make signing-list and reselect it.')
    digest, _ = certificate(identity)
    expected = read_pin(ROOT / 'config/macos-signing.json') if channel == 'release' else (digest if explicit else local.get('certificateSHA256'))
    require(digest == expected, 'Signing certificate differs from its stored SHA-256 pin.')
    return identity, digest

def update_config():
    data = json.loads((ROOT / 'config/updates.json').read_text())
    require(set(data) == {'publicKey', 'github', 'tagPrefix', 'deltas'}, 'Unexpected update configuration fields.')
    key = data['publicKey'].strip()
    try:
        decoded = base64.b64decode(key, validate=True)
    except ValueError as exc:
        raise BuildError('Invalid update public key.') from exc
    require(len(decoded) == 32, 'Commit the public key from mygo keygen to config/updates.json before a release build.')
    repo = os.environ.get('GITHUB_REPOSITORY', '') or data['github']
    require(re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', repo or '') is not None,
            'Set the verified public GitHub repository in config/updates.json (or GITHUB_REPOSITORY in CI).')
    require(not data['github'] or data['github'] == repo, 'CI repository differs from committed update repository.')
    require(data['tagPrefix'] == 'v' and data['deltas'] == 0, 'Only full updates with v tags are configured.')
    require(os.environ.get('MYGO_UPDATER_PRIVATE_KEY', '').strip(), 'MYGO_UPDATER_PRIVATE_KEY is required for release; unsigned updates are never published.')
    return dict(data, publicKey=key, github=repo)

def source_commit():
    try:
        commit = run('git', 'rev-parse', 'HEAD', capture=True)
    except BuildError:
        return 'unknown'
    dirty = run('git', 'status', '--porcelain', '--untracked-files=normal', capture=True)
    return commit + ('-dirty' if dirty else '')

def metadata(channel):
    name, bundle = identity_for(channel)
    return {'schema': 1, 'name': name, 'bundleID': bundle, 'version': version(), 'channel': channel,
            'executor': 'ci' if os.environ.get('GITHUB_ACTIONS') == 'true' else 'local',
            'commit': source_commit(), 'target': TARGET, 'source': 'source-build'}

def stable_requirement(output, bundle_id, identity):
    lines = [line.strip() for line in output.splitlines() if line.strip().startswith('designated => ')]
    if len(lines) != 1:
        return False
    expression = lines[0].removeprefix('designated => ')
    identifier = r'identifier (?:"' + re.escape(bundle_id) + r'"|' + re.escape(bundle_id) + r')'
    fixed = identifier + r' and certificate (?:leaf|root) = H"' + re.escape(identity) + r'"'
    if re.fullmatch(fixed, expression, re.I):
        return True
    # Apple's documented Developer ID requirement. Verify the exact conjunction
    # shape, and separately pin the leaf certificate bytes in verify_bundle.
    developer_id = (identifier + r' and anchor apple generic and certificate 1\[field\.1\.2\.840\.113635\.100\.6\.2\.6\] /\* exists \*/'
                    r' and certificate leaf\[field\.1\.2\.840\.113635\.100\.6\.1\.13\] /\* exists \*/'
                    r' and certificate leaf\[subject\.OU\] = "[A-Z0-9]{10}"')
    return re.fullmatch(developer_id, expression) is not None

def verify_bundle(app, expected, certificate_sha='', identity=''):
    require(app.is_dir() and not app.is_symlink(), f'Bundle missing: {app}')
    info = plistlib.loads((app / 'Contents/Info.plist').read_bytes())
    require(info.get('CFBundleIdentifier') == expected['bundleID'], 'Bundle identifier mismatch.')
    require(info.get('CFBundleShortVersionString') == expected['version'], 'Bundle version mismatch.')
    recorded = json.loads((app / 'Contents/Resources/build-info.json').read_text())
    require(recorded == expected, 'Build metadata mismatch.')
    exe = app / 'Contents/MacOS' / info['CFBundleExecutable']
    require(run('lipo', '-archs', exe, capture=True) == 'arm64', 'Unexpected executable architecture.')
    run('codesign', '--verify', '--deep', '--strict', app, capture=True)
    if certificate_sha:
        for target in [app, exe]:
            with tempfile.TemporaryDirectory(prefix='gopad-verify-') as temp:
                prefix = str(Path(temp) / 'cert')
                run('codesign', '--display', '--extract-certificates=' + prefix, target, capture=True)
                require(hashlib.sha256(Path(prefix + '0').read_bytes()).hexdigest() == certificate_sha,
                        'Final code was signed by an unexpected certificate.')
            # Evaluate an explicit certificate + identifier requirement. This accepts
            # both Developer ID and local fixed identities, never an ad-hoc cdhash.
            requirement = f'identifier "{expected["bundleID"]}" and certificate leaf = H"{identity}"'
            run('codesign', '--verify', '--strict', '-R=' + requirement, target, capture=True)
            result = subprocess.run(['codesign', '--display', '--requirements', '-', str(target)], text=True, capture_output=True)
            require(result.returncode == 0 and stable_requirement(result.stdout + '\n' + result.stderr, expected['bundleID'], identity),
                    'Unexpected designated requirement; expected a fixed certificate identity or Developer ID conjunction.')

def build(channel, package=False):
    require(sys.platform == 'darwin', 'Build macOS bundles on macOS.')
    require(run('uname', '-m', capture=True) == 'arm64', 'This release currently supports verified macOS arm64 only.')
    sign, cert = signing(channel)
    updates = update_config() if channel == 'release' else None
    meta = metadata(channel)
    if channel == 'release':
        require(meta['commit'] != 'unknown' and not meta['commit'].endswith('-dirty'), 'Release builds require a clean committed source tree.')
    (ROOT / 'build').mkdir(exist_ok=True)
    cfg = json.loads((ROOT / 'mygo.json').read_text())
    cfg.update(name=meta['name'], identifier=meta['bundleID'], version=meta['version'], main=str(ROOT))
    cfg['slug'] = 'gopad' if channel == 'release' else 'gopad-dev'
    if channel != 'release':
        icon_dir = ROOT / 'build/icons-dev'
        run(sys.executable, ROOT / 'scripts/dev_icon.py', '--input', ROOT / 'assets/gopad.png',
            '--output-dir', icon_dir, '--name', 'gopad-dev', '--formats', 'png,icns', '--force')
        cfg['icon'], cfg['macos']['icon'] = str(icon_dir / 'gopad-dev.png'), str(icon_dir / 'gopad-dev.icns')
    else:
        cfg['icon'], cfg['macos']['icon'] = str(ROOT / 'assets/gopad.png'), str(ROOT / 'assets/gopad.icns')
    cfg['macos']['signingIdentity'] = sign
    cfg['macos']['minimumSystemVersion'] = '12.0'
    cfg['macos']['infoPlist'] = {'CFBundleDevelopmentRegion': 'en', 'CFBundleLocalizations': ['en', 'zh-Hans'],
                               'GopadChannel': channel, 'GopadCommit': meta['commit'], 'GopadTarget': TARGET}
    if updates:
        cfg['updates'] = updates
    else:
        cfg.pop('updates', None)
    if channel == 'release' and os.environ.get('GOPAD_NOTARY_PROFILE'):
        cfg['macos']['notarize'] = {'keychainProfile': os.environ['GOPAD_NOTARY_PROFILE']}
        if os.environ.get('GOPAD_SIGNING_KEYCHAIN'):
            cfg['macos']['notarize']['keychain'] = os.environ['GOPAD_SIGNING_KEYCHAIN']
    with tempfile.TemporaryDirectory(prefix='.build-', dir=ROOT / 'build') as tmp:
        temporary = Path(tmp)
        cfg['out'] = str(temporary / 'output')
        record = temporary / 'build-info.json'
        record.write_text(json.dumps(meta, indent=2) + '\n')
        cfg['resources'] = [str(record), cfg['icon']]
        if updates:
            notes = run(sys.executable, ROOT / 'scripts/release-notes.py', updates['github'], 'v' + meta['version'], capture=True)
            changelog = temporary / 'CHANGELOG.md'
            changelog.write_text('## ' + meta['version'] + '\n\n' + notes + '\n')
            cfg['updates']['changelog'] = str(changelog)
        (temporary / 'mygo.json').write_text(json.dumps(cfg, indent=2) + '\n')
        inject = {'Version': meta['version'], 'Channel': channel, 'Executor': meta['executor'], 'Commit': meta['commit'],
                  'Repository': updates['github'] if updates else '', 'PublicKey': updates['publicKey'] if updates else ''}
        env = dict(os.environ)
        # The application keeps MyGo's feed/key, but the one distributable ZIP is
        # signed below. Do not ask the stock CLI to produce a second tar archive.
        if updates:
            env.pop('MYGO_UPDATER_PRIVATE_KEY', None)
        env['MYGO_LDFLAGS'] = ' '.join('-X github.com/knothhe/gopad/internal/buildinfo.' + k + '=' + v for k, v in inject.items())
        # Debug builds avoid production-only features/timestamp requirements for local self-signed Dev certificates.
        args = [sys.executable, ROOT / 'scripts/go.py', 'run', 'github.com/egoist/mygo/cmd/mygo', 'build', '-platform', 'darwin/arm64']
        if channel != 'release':
            args.append('-debug')
        if not (channel == 'release' and os.environ.get('GOPAD_NOTARY_PROFILE')):
            args.append('-skip-dmg')
        run(*args, temporary, env=env)
        if channel == 'release':
            require(source_commit() == meta['commit'], 'Build changed the source checkout; refusing to publish those artifacts.')
        output = temporary / 'output' / TARGET
        require(not list(output.glob('*.tar.gz')), 'Unexpected legacy tar archive; only the shared ZIP may be distributed.')
        app = output / (meta['name'] + '.app')
        verify_bundle(app, meta, cert, sign)
        if package:
            filename = f'gopad-{meta["version"]}-macos-arm64.zip' if channel == 'release' else f'gopad-dev-{meta["version"]}-macos-arm64.zip'
            archive = output / filename
            run('ditto', '-c', '-k', '--sequesterRsrc', '--keepParent', app, archive)
            with tempfile.TemporaryDirectory(prefix='gopad-extract-') as extracted:
                run('ditto', '-x', '-k', archive, extracted)
                verify_bundle(Path(extracted) / app.name, meta, cert, sign)
            if updates:
                run('go', 'run', './scripts/prepare-update', '--archive', archive,
                    '--version', meta['version'], '--repo', updates['github'],
                    '--public-key', updates['publicKey'], '--notes', changelog)
        destination = ROOT / 'build' / channel / TARGET
        destination.parent.mkdir(parents=True, exist_ok=True)
        require(not destination.is_symlink(), 'Build destination cannot be a symbolic link.')
        backup = destination.with_name(destination.name + '.previous')
        require(not backup.exists(), f'An earlier recovery backup exists: {backup}')
        if destination.exists():
            destination.rename(backup)
        try:
            output.rename(destination)
        except BaseException:
            if backup.exists():
                backup.rename(destination)
            raise
        if backup.exists():
            shutil.rmtree(backup)
    print(f'Built {channel}: {destination / (meta["name"] + ".app")}')
    if channel == 'preview':
        print('Preview is ad-hoc signed for local QA; installation and release updates are disabled.')
    return destination, meta, cert, sign

def dev_processes(destination):
    """Find only executables inside the exact installation, without argv matching."""
    prefix = str(destination / 'Contents/MacOS') + '/'
    processes = run('ps', '-axo', 'pid=,comm=', capture=True).splitlines()
    matches = []
    for line in processes:
        fields = line.strip().split(None, 1)
        if len(fields) == 2 and fields[1].startswith(prefix):
            matches.append(int(fields[0]))
    return matches

def stop_dev(destination):
    pids = dev_processes(destination)
    if not pids:
        return False
    print(f'Stopping running Dev app: {destination}')
    for pid in pids:
        try:
            os.kill(pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
    deadline = time.monotonic() + 10
    while dev_processes(destination):
        require(time.monotonic() < deadline,
                'The target Dev app did not exit within 10 seconds; installation was left unchanged.')
        time.sleep(0.1)
    return True

def install(destination):
    dest = no_symlink(destination).expanduser()
    require(dest.name == 'gopad Dev.app', 'Dev installation must use the exact name gopad Dev.app.')
    data = Path.home() / 'Library/Application Support/gopad Dev'
    require(dest != data and dest not in data.parents and data not in dest.parents, 'Install target overlaps Dev data.')
    if dest.exists():
        info = plistlib.loads((dest / 'Contents/Info.plist').read_bytes())
        require(info.get('CFBundleIdentifier') == 'dev.knothhe.gopad.dev', 'Refusing to replace an unrelated or release application.')
        receipt = dest / 'Contents/Resources/build-info.json'
        require(receipt.is_file() and json.loads(receipt.read_text()).get('channel') == 'dev', 'Existing bundle has no verified Dev build identity.')
    output, meta, cert, identity = build('dev')
    source = output / 'gopad Dev.app'
    dest.parent.mkdir(parents=True, exist_ok=True)
    no_symlink(dest.parent)
    with tempfile.TemporaryDirectory(prefix='.gopad-install-', dir=dest.parent) as temp:
        stage = Path(temp) / dest.name
        run('ditto', source, stage)
        verify_bundle(stage, meta, cert, identity)
        # Finish copying and verification before interrupting the installed app.
        restart = stop_dev(dest)
        backup = Path(temp) / 'previous.app'
        try:
            if dest.exists():
                dest.rename(backup)
            try:
                stage.rename(dest)
            except BaseException:
                if backup.exists():
                    backup.rename(dest)
                raise
            register = Path('/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister')
            run(register, '-f', dest)
        finally:
            # Also reopen the restored old installation if replacement fails.
            if restart and dest.exists():
                run('open', dest)
                print(f'Restarted Dev app: {dest}')
    print(f'Installed source-built Dev app: {dest}\nPreferences preserved: {data}')

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('action', choices=['build', 'package', 'install', 'signing-list', 'signing-config', 'verify-tag'])
    p.add_argument('--channel', choices=['dev', 'preview', 'release'], default=os.environ.get('CHANNEL', 'dev'))
    p.add_argument('--identity')
    p.add_argument('--destination', default=str(Path.home() / 'Applications/gopad Dev.app'))
    p.add_argument('--tag')
    args = p.parse_args()
    if args.action == 'signing-list':
        identities = available_identities()
        if not identities:
            print('No paired Code Signing identity found. See docs/build-release.md for Keychain Access setup.')
        for identity, label in identities.items():
            digest, dates = certificate(identity)
            print(f'{identity}  {label}\n  SHA256 {digest}\n  {dates}')
    elif args.action == 'signing-config':
        identity = (args.identity or '').upper()
        require(identity in available_identities(), 'Choose an exact SHA1 from make signing-list; no identity is chosen automatically.')
        digest, _ = certificate(identity)
        require(not LOCAL.is_symlink(), 'Local signing configuration cannot be a symbolic link.')
        LOCAL.write_text(json.dumps({'identity': identity, 'certificateSHA256': digest}, indent=2) + '\n')
        LOCAL.chmod(0o600)
        print('Saved this repository’s local identity reference. No private key was exported.')
    elif args.action == 'verify-tag':
        require(args.tag == 'v' + version(), 'Tag must exactly match v + VERSION.')
        require(run('git', 'rev-parse', 'HEAD', capture=True) == run('git', 'rev-parse', '--verify', f'refs/tags/{args.tag}^{{commit}}', capture=True), 'Checkout does not match requested tag.')
        require(not run('git', 'status', '--porcelain', capture=True), 'Tag checkout must be clean.')
    elif args.action == 'install':
        require(args.channel == 'dev', 'make install only installs an isolated Dev source build; use the verified Release package for production.')
        install(Path(args.destination).expanduser())
    else:
        build(args.channel, args.action == 'package')

if __name__ == '__main__':
    try:
        main()
    except (BuildError, ReleaseError, OSError, ValueError, KeyError) as error:
        print(f'ERROR: {error}', file=sys.stderr)
        sys.exit(1)
