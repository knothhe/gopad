#!/usr/bin/env python3
"""Validate and upload this repository's macOS and MyGo signing secrets."""
from __future__ import annotations
import argparse
import base64
import getpass
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shlex
import shutil
import ssl
import subprocess
import sys
import tempfile
import time

sys.dont_write_bytecode = True
from macos import ROOT, BuildError, no_symlink, read_pin, require
from release import ReleaseError, github_repo
import signing_keychain

SECRET_NAMES = ('MACOS_SIGNING_CERTIFICATE', 'MACOS_SIGNING_CERTIFICATE_PASSWORD',
                'MYGO_UPDATER_PRIVATE_KEY')
ED25519_PRIVATE_PREFIX = bytes.fromhex('302e020100300506032b657004220420')
ED25519_PUBLIC_PREFIX = bytes.fromhex('302a300506032b6570032100')


def command(*args, input=None):
    """Keep secret stdin, security arguments, and subprocess diagnostics private."""
    result = subprocess.run([str(arg) for arg in args], cwd=ROOT, input=input,
                            capture_output=True)
    if result.returncode:
        raise BuildError(f'{Path(str(args[0])).name} {args[1]} failed '
                         f'(exit {result.returncode}); arguments and output omitted.')
    return result.stdout


def private_file(value, label):
    require(bool(str(value).strip()), f'{label} path is required.')
    path = no_symlink(Path(value).expanduser())
    require(path != ROOT.resolve() and ROOT.resolve() not in path.parents,
            f'Keep {label} outside the source repository.')
    require(path.is_file(), f'{label} must be an existing regular file.')
    data = path.read_bytes()
    require(bool(data), f'{label} is empty.')
    return data


def updater_public_key(text):
    """Validate MyGo's seed + public-key format against the derived Ed25519 key."""
    try:
        key = base64.b64decode(text.strip(), validate=True)
    except (ValueError, TypeError) as error:
        raise BuildError('Invalid MyGo updater private key; expected Base64 keygen output.') from error
    require(len(key) == 64, 'MyGo updater private key must decode to 64 bytes.')
    try:
        public_der = command('openssl', 'pkey', '-inform', 'DER', '-pubout', '-outform', 'DER',
                             input=ED25519_PRIVATE_PREFIX + key[:32])
    except BuildError as error:
        raise BuildError('Could not derive the updater public key. Put OpenSSL with Ed25519 '
                         'support (for example, Homebrew OpenSSL 3) on PATH and retry.') from error
    require(public_der == ED25519_PUBLIC_PREFIX + key[32:],
            'MyGo updater private key has an inconsistent seed and public key.')
    return base64.b64encode(key[32:]).decode(), base64.b64encode(key)


def certificate_in_keychain(keychain, identity):
    pem = command('security', 'find-certificate', '-a', '-p', keychain)
    for block in re.findall(b'-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----', pem, re.S):
        der = base64.b64decode(b''.join(block.splitlines()[1:-1]), validate=True)
        if hashlib.sha1(der).hexdigest().upper() != identity:
            continue
        description = command('openssl', 'x509', '-noout', '-text', input=block).decode()
        eku = re.search(r'X509v3 Extended Key Usage:[^\n]*\n\s+([^\n]+)', description)
        require(eku is not None and 'Code Signing' in eku.group(1).split(', '),
                'Certificate has no Code Signing extended key usage.')
        dates = command('openssl', 'x509', '-noout', '-dates', input=block).decode()
        values = dict(line.split('=', 1) for line in dates.strip().splitlines())
        require(ssl.cert_time_to_seconds(values['notBefore']) <= time.time() <=
                ssl.cert_time_to_seconds(values['notAfter']),
                'Signing certificate is expired or not yet valid.')
        return hashlib.sha256(der).hexdigest()
    raise BuildError('Imported signing identity has no matching public certificate.')


def inspect_p12(data, password):
    """Test macOS import in a temporary keychain; restore the user's search list."""
    original = shlex.split(command('security', 'list-keychains', '-d', 'user').decode())
    temporary = Path(tempfile.mkdtemp(prefix='gopad-signing-upload-'))
    temporary.chmod(0o700)
    keychain = temporary / 'signing.keychain-db'
    attempted = False
    created = False
    cleanup_failures = []
    try:
        p12 = temporary / 'certificate.p12'
        p12.write_bytes(data)
        p12.chmod(0o600)
        # A PKCS#12 can hide unrelated or unpaired private keys even when the
        # Code Signing policy reports one identity. Never upload those keys.
        parse_args = ('openssl', 'pkcs12', '-in', p12, '-nodes', '-nocerts', '-passin', 'stdin')
        password_input = password.encode() + b'\n'
        try:
            private_pem = command(*parse_args, input=password_input)
        except BuildError:
            # Keychain Access exports can use RC2, available through OpenSSL 3's
            # legacy provider. This does not change or repackage the source P12.
            if not command('openssl', 'version').startswith(b'OpenSSL 3.'):
                raise
            private_pem = command(*parse_args, '-legacy', input=password_input)
        require(len(re.findall(rb'-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----', private_pem)) == 1,
                'P12 must contain exactly one private key; unrelated keys cannot be uploaded.')
        keychain_password = secrets.token_urlsafe(32)
        attempted = True
        command('security', 'create-keychain', '-p', keychain_password, keychain)
        created = True
        command('security', 'unlock-keychain', '-p', keychain_password, keychain)
        command('security', 'import', p12, '-k', keychain, '-f', 'pkcs12', '-P', password,
                '-T', '/usr/bin/codesign')
        identities = set(re.findall(rb'\)\s+([A-Fa-f0-9]{40})\s+"',
                                    command('security', 'find-identity', '-p', 'codesigning', keychain)))
        require(len(identities) == 1, 'P12 must contain exactly one paired Code Signing identity.')
        return certificate_in_keychain(keychain, next(iter(identities)).decode().upper())
    finally:
        if attempted:
            try:
                command('security', 'list-keychains', '-d', 'user', '-s', *original)
            except (BuildError, OSError) as error:
                cleanup_failures.append(str(error))
            if created or keychain.exists():
                try:
                    command('security', 'delete-keychain', keychain)
                except (BuildError, OSError) as error:
                    cleanup_failures.append(str(error))
        try:
            shutil.rmtree(temporary)
        except OSError:
            cleanup_failures.append('Could not remove private temporary signing directory.')
        if cleanup_failures:
            raise BuildError('Signing cleanup incomplete; no secrets uploaded: ' + '; '.join(cleanup_failures))


def save_public_config(pin_path, pin_before, digest, updates_path, updates_before, updates, public_key):
    # Save the validated identities before uploading so partial uploads can be retried
    # without treating this same identity as an unknown pre-existing secret.
    for path, before in [(pin_path, pin_before), (updates_path, updates_before)]:
        no_symlink(path)
        require((path.read_bytes() if path.exists() else None) == before,
                'Public signing configuration changed during validation; run the command again.')
    if pin_before is None:
        with pin_path.open('x') as stream:
            stream.write(json.dumps({'schema': 1, 'certificateSHA256': digest}, indent=2) + '\n')
    if not updates['publicKey'].strip():
        updated = dict(updates, publicKey=public_key)
        descriptor, name = tempfile.mkstemp(prefix='.updates-', suffix='.json', dir=updates_path.parent)
        try:
            with os.fdopen(descriptor, 'w') as stream:
                stream.write(json.dumps(updated, indent=2) + '\n')
            Path(name).chmod(updates_path.stat().st_mode & 0o777)
            Path(name).replace(updates_path)
        finally:
            Path(name).unlink(missing_ok=True)


def load_context(repo=''):
    updates_path = no_symlink(ROOT / 'config/updates.json')
    pin_path = no_symlink(ROOT / 'config/macos-signing.json')
    updates_before = updates_path.read_bytes()
    updates = json.loads(updates_before)
    require(isinstance(updates, dict) and set(updates) == {'publicKey', 'github', 'tagPrefix', 'deltas'} and
            isinstance(updates['publicKey'], str) and isinstance(updates['github'], str) and
            updates['tagPrefix'] == 'v' and updates['deltas'] == 0,
            'Invalid public update configuration; see docs/build-release.md.')
    pin_before = pin_path.read_bytes() if pin_path.exists() else None
    old_pin = read_pin(pin_path) if pin_before is not None else ''
    origin = github_repo(command('git', 'remote', 'get-url', 'origin').decode().strip())
    repo = repo.strip() or origin
    require(re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', repo) is not None and
            repo == origin and repo == updates['github'],
            'Upload repository must match origin and config/updates.json github.')
    return {'repo': repo, 'updates_path': updates_path, 'updates_before': updates_before,
            'updates': updates, 'pin_path': pin_path, 'pin_before': pin_before, 'old_pin': old_pin}


def load_remote(context):
    # Bind every request to the same host validated from origin. GH_HOST must
    # never redirect signing material to a same-named Enterprise repository.
    repo = context['repo']
    github_target = 'github.com/' + repo
    command('gh', 'auth', 'status', '--hostname', 'github.com')
    canonical_repo = command('gh', 'repo', 'view', github_target, '--json', 'nameWithOwner',
                             '--jq', '.nameWithOwner').decode().strip()
    require(canonical_repo == repo, 'GitHub repository identity differs from the upload target.')
    context['existing'] = set(command('gh', 'secret', 'list', '--repo', github_target, '--json', 'name',
                                     '--jq', '.[].name').decode().splitlines())


def show_context(context):
    print(f'目标仓库: github.com/{context["repo"]}\n'
          f'发行证书 SHA256: {context["old_pin"] or "（未配置）"}\n'
          f'更新公钥: {context["updates"]["publicKey"].strip() or "（未配置）"}')
    for name in SECRET_NAMES:
        print(f'{name}: {"已存在" if name in context["existing"] else "未配置"}')


def require_known_remote(context):
    require(context['old_pin'] or not context['existing'].intersection(SECRET_NAMES[:2]),
            'Repository already has certificate secrets but no local certificate pin. Restore the original public pin before uploading.')
    require(context['updates']['publicKey'].strip() or SECRET_NAMES[2] not in context['existing'],
            'Repository already has an updater secret but no configured public key. Restore the original public key before uploading.')


def validate_updater(data, context):
    public_key, normalized = updater_public_key(data)
    require(not context['updates']['publicKey'].strip() or context['updates']['publicKey'].strip() == public_key,
            'Updater private key differs from the configured public key; refusing key rotation.')
    return public_key, normalized


def finish_upload(context, p12_data, password, digest, public_key, normalized_private_key,
                  dry_run=False, confirm=False):
    repo, old_pin, updates = context['repo'], context['old_pin'], context['updates']
    require(not old_pin or old_pin == digest,
            'Signing certificate differs from config/macos-signing.json; refusing certificate rotation.')
    if 'existing' not in context:
        load_remote(context)
    existing = context['existing']
    print(f'Repository: {repo}\nStored certificate SHA256: {old_pin or "(unset)"}\n'
          f'P12 certificate SHA256: {digest}\nUpdater public key: {public_key}')
    for name in SECRET_NAMES:
        print(f'{name}: {"present, will update" if name in existing else "missing, will create"}')
    require_known_remote(context)
    print('公开配置: ' + ('保留已有证书指纹' if old_pin else '新增 config/macos-signing.json') + '；' +
          ('保留已有更新公钥' if updates['publicKey'].strip() else '写入 config/updates.json 更新公钥'))
    if dry_run:
        print('Dry run complete: no secrets uploaded and no public configuration written.')
        return repo
    if confirm:
        try:
            accepted = input('确认向上述仓库上传这三项 Secrets？[y/N]: ').strip().lower() in ('y', 'yes')
        except (EOFError, KeyboardInterrupt):
            accepted = False
        if not accepted:
            print('已取消，未上传，也未写入公开配置。')
            return None
    uploaded = []
    try:
        save_public_config(context['pin_path'], context['pin_before'], digest,
                           context['updates_path'], context['updates_before'], updates, public_key)
        for name, value in zip(SECRET_NAMES, (base64.b64encode(p12_data), password.encode(), normalized_private_key)):
            try:
                command('gh', 'secret', 'set', name, '--repo', 'github.com/' + repo, input=value)
            except (BuildError, OSError, KeyboardInterrupt) as error:
                raise BuildError(f'Upload failed for {name}. Successfully uploaded: {", ".join(uploaded) or "none"}. '
                                 'The failed request may have reached GitHub. Public configuration is retained; '
                                 'retry with the same signing files. Tool output omitted.') from error
            uploaded.append(name)
            print(f'Uploaded {name}.')
    except KeyboardInterrupt as error:
        raise BuildError(f'Signing upload interrupted. Successfully uploaded: {", ".join(uploaded) or "none"}. '
                         'Public configuration may have been saved; retain it and retry with the same signing files.') from error
    print(f'Uploaded all three signing secrets to {repo}. Commit config/macos-signing.json and '
          'config/updates.json before releasing. CI import and signing remain the final remote verification.')
    return repo


def upload(p12, updater_key, repo='', dry_run=False):
    """Explicit material paths keep the existing password-only interaction."""
    require(sys.platform == 'darwin', 'Run signing-upload on macOS to verify P12 import compatibility.')
    p12_data = private_file(p12, 'P12 certificate')
    updater_data = private_file(updater_key, 'MyGo updater private key')
    context = load_context(repo)
    public_key, normalized_private_key = validate_updater(updater_data, context)
    require(sys.stdin.isatty(), 'P12 password requires an interactive terminal; run make signing-upload in your terminal.')
    password = getpass.getpass('P12 export password: ')
    require(bool(password), 'P12 export password cannot be empty; CI requires a password secret.')
    digest = inspect_p12(p12_data, password)
    return finish_upload(context, p12_data, password, digest, public_key, normalized_private_key, dry_run)


class Cancelled(Exception):
    pass


def prompt_path(label, default=None):
    answer = input(label + (f' [{default}]' if default is not None else '') + '（q 取消）: ').strip()
    if answer.lower() == 'q' or (not answer and default is None):
        raise Cancelled()
    answer = answer or str(default)
    # Accept literal paths with spaces, shell quotes, and files dragged from Finder.
    if not Path(answer).expanduser().exists():
        try:
            parts = shlex.split(answer)
        except ValueError:
            raise BuildError('文件路径的引号不完整，请重新运行并输入一个完整路径。') from None
        if len(parts) == 1:
            answer = parts[0]
    return str(Path(answer).expanduser())


def local_identities(context):
    remembered = ''
    local = no_symlink(ROOT / '.gopad-local.json')
    if local.is_file():
        try:
            remembered = json.loads(local.read_text()).get('identity', '').upper()
        except (ValueError, AttributeError):
            print('本机 Dev 身份记录无法读取，请使用 make signing-config 重新选择。')
    if remembered:
        print(f'本机已选 Dev 身份 SHA1: {remembered}')
    candidates = []
    for candidate in signing_keychain.list_identities(command):
        try:
            digest = certificate_in_keychain(candidate['keychain'], candidate['identity'])
        except BuildError as error:
            print(f'跳过不可用身份 {candidate["identity"]}: {error}')
            continue
        labels = []
        if digest == context['old_pin']:
            labels.append('匹配发行指纹')
        if candidate['identity'] == remembered:
            labels.append('本机 Dev 已选')
        candidates.append(dict(candidate, digest=digest))
        print(f'{len(candidates)}. {candidate["label"]}' + (f'（{"；".join(labels)}）' if labels else '') +
              f'\n   SHA1: {candidate["identity"]}\n   SHA256: {digest}')
    if not candidates:
        print('未找到可用本机代码签名身份。可在“钥匙串访问”导入已有身份，或通过“证书助理”→“创建证书”\n'
              '选择 Self Signed Root / Code Signing，保留配对私钥后重试；也可使用已有 P12。')
    return candidates


def wizard(p12='', updater_key='', repo='', dry_run=False):
    require(sys.platform == 'darwin', 'Run signing-upload on macOS to verify P12 import compatibility.')
    require(sys.stdin.isatty(), '交互配置需要终端。请在终端运行 make signing-upload；'
            '也可指定 SIGNING_P12="/path/certificate.p12" UPDATER_KEY="/path/mygo-update.key"。')
    try:
        context = load_context(repo)
        load_remote(context)
        show_context(context)
        candidates = local_identities(context)
        require_known_remote(context)
        selected = None
        if not str(p12).strip():
            print('p. 使用已有 P12 文件\nq. 取消')
            while True:
                choice = input('选择要用于发行的本机身份编号，或输入 p / q: ').strip().lower()
                if choice in ('', 'q'):
                    raise Cancelled()
                if choice == 'p':
                    p12 = prompt_path('P12 文件路径')
                    break
                if choice.isdigit() and 1 <= int(choice) <= len(candidates):
                    selected = candidates[int(choice) - 1]
                    require(not context['old_pin'] or context['old_pin'] == selected['digest'],
                            'Signing certificate differs from config/macos-signing.json; refusing certificate rotation.')
                    print(f'将仅导出所选身份 {selected["identity"]} 的证书及配对私钥，系统可能请求钥匙串授权。')
                    break
                print('请输入列表中的编号、p 或 q。')
        # Validate the updater first, before asking Keychain to export any private key.
        if not str(updater_key).strip():
            default = Path.home() / 'Library/Application Support/mygo/update-keys/mygo-update.key'
            updater_key = prompt_path('MyGo 更新私钥文件路径', default if default.is_file() else None)
        if not Path(updater_key).expanduser().is_file():
            print('如尚未生成更新密钥，请先运行：go run github.com/egoist/mygo/cmd/mygo keygen；'
                  '在仓库外保管生成的 mygo-update.key 和离线备份。')
        updater_data = private_file(updater_key, 'MyGo updater private key')
        public_key, normalized_private_key = validate_updater(updater_data, context)
        if selected is not None:
            p12_data, password = signing_keychain.export_identity(selected['identity'], selected['keychain'], command)
        else:
            p12_data = private_file(p12, 'P12 certificate')
            password = getpass.getpass('P12 导出密码（输入不回显）: ')
            require(bool(password), 'P12 export password cannot be empty; CI requires a password secret.')
        digest = inspect_p12(p12_data, password)
        if selected is not None:
            require(digest == selected['digest'], 'Exported certificate differs from the selected identity.')
    except (Cancelled, EOFError, KeyboardInterrupt):
        print('\n已取消，未上传，也未写入公开配置。')
        return None
    return finish_upload(context, p12_data, password, digest, public_key,
                         normalized_private_key, dry_run, confirm=True)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--p12', default='', help='Optional P12 path; omitted values are collected interactively')
    parser.add_argument('--updater-key', default='', help='Optional MyGo private-key path outside this repository')
    parser.add_argument('--repo', default='', help='GitHub owner/repository (must match origin and update configuration)')
    parser.add_argument('--dry-run', action='store_true', help='Validate and preview without uploading or writing public configuration')
    args = parser.parse_args(argv)
    if not args.p12.strip() or not args.updater_key.strip():
        return wizard(args.p12, args.updater_key, args.repo, args.dry_run)
    return upload(args.p12, args.updater_key, args.repo, args.dry_run)


if __name__ == '__main__':
    try:
        main()
    except (BuildError, ReleaseError, OSError, ValueError, KeyError, EOFError, KeyboardInterrupt) as error:
        print(f'ERROR: {error or "Signing upload cancelled."}', file=sys.stderr)
        sys.exit(1)
