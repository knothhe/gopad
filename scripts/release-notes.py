#!/usr/bin/env python3
"""Read-only notes for an existing local tag; no gh or remote access."""
import argparse
import json
import os
from pathlib import Path
import re
import sys

# Importing the helper must not create __pycache__ in a read-only notes command.
sys.dont_write_bytecode = True
from release import ReleaseError, git, parse, release_notes, require


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('repo', help='GitHub owner/repo')
    parser.add_argument('tag', help='Existing local tag; tag_prefix comes from .release.json (default: v)')
    args = parser.parse_args()
    require(re.fullmatch(r'[\w.-]+/[\w.-]+', args.repo, flags=re.ASCII) is not None,
            'Expected GitHub owner/repo.')
    os.chdir(git('rev-parse', '--show-toplevel'))
    config = json.loads(Path('.release.json').read_text()) if Path('.release.json').exists() else {}
    require(isinstance(config, dict), '.release.json must contain an object.')
    prefix = config.get('tag_prefix', 'v')
    require(isinstance(prefix, str) and not prefix.startswith('-') and args.tag.startswith(prefix),
            'Tag must match the configured tag_prefix.')
    version = args.tag[len(prefix):]
    parse(version)
    git('check-ref-format', 'refs/tags/' + args.tag)
    source_head = git('rev-parse', '--verify', 'refs/tags/' + args.tag + '^{commit}')
    sys.stdout.write(release_notes(args.repo, prefix, version, source_head, args.tag))


if __name__ == '__main__':
    try:
        main()
    except (ReleaseError, OSError, ValueError, TypeError) as error:
        print(f'ERROR: {error}', file=sys.stderr)
        sys.exit(1)
