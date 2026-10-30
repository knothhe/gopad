#!/usr/bin/env python3
"""Run Go with the narrowly pinned MyGo ZIP extraction compatibility patch."""
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
MYGO_VERSION = 'v0.7.3'
UPDATER_SHA256 = '590c0e03913ec94c1e29c478ae62d348f5805da7d54f7c8dfb6a89b02b778219'


def patched_source(source):
    if hashlib.sha256(source.encode()).hexdigest() != UPDATER_SHA256:
        raise ValueError('MyGo updater source changed; review the ZIP compatibility patch before building.')
    old = '\terr = update.ExtractArchive(f, unpacked)'
    new = ('\tif updatearchive.IsZIP(f) {\n'
           '\t\terr = updatearchive.ExtractZIP(f, unpacked)\n'
           '\t} else {\n' + old + '\n\t}')
    import_line = '\t"github.com/egoist/mygo/internal/update"'
    if source.count(old) != 1 or source.count(import_line) != 1:
        raise ValueError('MyGo updater patch context is ambiguous.')
    return source.replace(old, new).replace(import_line, import_line + '\n\t"github.com/knothhe/gopad/pkg/updatearchive"')


def main():
    flags = shlex.split(os.environ.get('GOFLAGS', ''))
    if any(flag in ('-overlay', '-modfile') or flag.startswith(('-overlay=', '-modfile=')) for flag in flags + sys.argv[1:]):
        raise ValueError('An existing Go overlay/modfile conflicts with the required MyGo ZIP compatibility patch.')
    config = json.loads((ROOT / 'mygo.json').read_text())
    if config.get('frontendDist'):
        raise ValueError('MyGo frontendDist creates a conflicting build overlay; merge overlays before enabling it.')
    result = subprocess.run(['go', 'list', '-m', '-json', 'github.com/egoist/mygo'], cwd=ROOT, capture_output=True, text=True, check=True)
    module = json.loads(result.stdout)
    if module.get('Version') != MYGO_VERSION or module.get('Replace'):
        raise ValueError('ZIP compatibility requires the reviewed, unmodified MyGo ' + MYGO_VERSION + '.')
    if not module.get('Dir'):
        downloaded = subprocess.run(['go', 'mod', 'download', '-json', 'github.com/egoist/mygo'], cwd=ROOT, capture_output=True, text=True, check=True)
        module = json.loads(downloaded.stdout)
        if module.get('Version') != MYGO_VERSION or not module.get('Dir'):
            raise ValueError('Cannot locate the pinned MyGo module source.')
    original = Path(module['Dir']) / 'updater.go'
    source = patched_source(original.read_text())
    (ROOT / 'build').mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='mygo-zip-overlay-', dir=ROOT / 'build') as temp:
        output = Path(temp)
        # Go refuses overlays inside GOMODCACHE. Keep the downloaded module
        # immutable, and select a private source copy through a temporary modfile.
        copied_module = output / 'mygo'
        shutil.copytree(original.parent, copied_module)
        modfile = output / 'go.mod'
        modfile.write_text((ROOT / 'go.mod').read_text() + '\nreplace github.com/egoist/mygo => ' + json.dumps(str(copied_module)) + '\n')
        shutil.copyfile(ROOT / 'go.sum', output / 'go.sum')
        patched = output / 'updater.go'
        patched.write_text(source)
        overlay = output / 'overlay.json'
        overlay.write_text(json.dumps({'Replace': {str(copied_module / 'updater.go'): str(patched)}}))
        env = dict(os.environ, GOFLAGS=shlex.join(flags + ['-modfile=' + str(modfile), '-overlay=' + str(overlay)]))
        return subprocess.call(['go', *sys.argv[1:]], cwd=ROOT, env=env)


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print('ERROR: ' + str(error), file=sys.stderr)
        sys.exit(1)
