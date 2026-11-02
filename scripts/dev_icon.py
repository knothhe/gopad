#!/usr/bin/env python3
"""Derive badged Dev icons without modifying the original application artwork."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import stat
import sys
import struct
import subprocess
import zlib
import tempfile


TOOL = "h2-desktop-client/dev-icon"
MANIFEST = ".dev-icon-manifest.json"
PNG_SIZES = (16, 24, 32, 48, 64, 128, 256, 512, 1024)
ICO_SIZES = (16, 20, 24, 32, 40, 48, 64, 128, 256)
ICNS_SIZES = (16, 32, 64, 128, 256, 512, 1024)
MAX_EDGE = 4096


class IconError(Exception):
    pass


def native_renderer(*args, request=None):
    if sys.platform != "darwin":
        raise IconError("Dev icon generation requires macOS and Xcode Command Line Tools.")
    renderer = Path(__file__).with_name("render-dev-icon.swift")
    result = subprocess.run(
        ["xcrun", "swift", str(renderer), *map(str, args)],
        input=json.dumps(request) if request is not None else None,
        text=True, capture_output=True,
    )
    if result.returncode:
        raise IconError(result.stderr.strip() or "Native Dev icon renderer failed.")
    return result.stdout


def parser():
    p = argparse.ArgumentParser(
        description="Add a bottom-right Dev badge and generate PNG/ICO/ICNS in a separate directory.",
        epilog="Requires macOS, Python >= 3.9 and Xcode Command Line Tools. Input artwork is never modified.",
    )
    p.add_argument("--input", required=True, type=Path, help="original square PNG, ICO or ICNS")
    p.add_argument("--output-dir", required=True, type=Path, help="dedicated Dev resource directory")
    p.add_argument("--formats", default="png,ico,icns", help="comma-separated png,ico,icns")
    p.add_argument("--sizes", help="PNG pixel sizes, e.g. 32,64,128,256; default: standard sizes up to source size, plus source size")
    p.add_argument("--name", default="app-icon-dev", help="output basename without extension (default: app-icon-dev)")
    p.add_argument("--allow-upscale", action="store_true", help="explicitly allow outputs larger than the source artwork")
    p.add_argument("--force", action="store_true", help="regenerate verified outputs owned by this script; preserve unrelated files")
    return p


def digest(data):
    return hashlib.sha256(data).hexdigest()


def checked_edge(width, height):
    if width != height or not 16 <= width <= MAX_EDGE:
        raise IconError(f"Artwork must be square and between 16 and {MAX_EDGE} pixels; got {width}x{height}.")


def read_source(source):
    data = source.read_bytes()
    if data.startswith(b"\x89PNG\r\n\x1a\n"):
        offset = 8
        while offset + 12 <= len(data):
            length = struct.unpack_from(">I", data, offset)[0]
            if data[offset + 4:offset + 8] == b"tEXt" and data[offset + 8:offset + 8 + length].startswith(TOOL.encode() + b"\0"):
                raise IconError("Input is already a generated Dev PNG; use the original artwork to avoid double badging.")
            offset += length + 12
    frames = json.loads(native_renderer("--inspect", source))
    if not frames:
        raise IconError("No usable icon representations found.")
    for edge in frames:
        checked_edge(edge, edge)
    # Container formats cannot carry the standalone PNG marker reliably.
    manifest = source.parent / MANIFEST
    if manifest.is_file() and not manifest.is_symlink():
        try:
            record = json.loads(manifest.read_text(encoding="utf-8"))
        except (OSError, ValueError):
            record = {}
        if isinstance(record, dict) and record.get("tool") == TOOL and isinstance(record.get("files"), dict) and source.name in record["files"]:
            raise IconError("Input belongs to generated Dev resources; use the original artwork.")
    return frames, digest(data)


def parse_options(args, largest):
    formats = [word.strip().lower() for word in args.formats.split(",")]
    if not formats or len(set(formats)) != len(formats) or any(word not in {"png", "ico", "icns"} for word in formats):
        raise IconError("--formats must contain distinct values from png,ico,icns.")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,79}", args.name):
        raise IconError("--name must be a filename basename using letters, numbers, dot, dash or underscore.")
    if args.sizes and "png" not in formats:
        raise IconError("--sizes only applies to PNG; include png in --formats.")
    if args.sizes is None:
        sizes = sorted({size for size in PNG_SIZES if size <= largest} | {largest})
    else:
        try:
            sizes = sorted({int(word.strip()) for word in args.sizes.split(",")})
        except ValueError as exc:
            raise IconError("--sizes must be comma-separated integers.") from exc
        for size in sizes:
            checked_edge(size, size)
    required = set(sizes if "png" in formats else [])
    if "ico" in formats:
        required.update(ICO_SIZES)
    if "icns" in formats:
        required.update(ICNS_SIZES)
    if max(required) > largest and not args.allow_upscale:
        raise IconError(
            f"Source has {largest}px artwork, but selected outputs require {max(required)}px. "
            "Use higher-resolution PNG/ICO/ICNS artwork, omit that format, or explicitly pass --allow-upscale."
        )
    return formats, sizes, required


def owned_outputs(output, force):
    if output.is_symlink() or (output.exists() and not output.is_dir()):
        raise IconError("Output must be a real directory, not a file or symbolic link.")
    if not output.exists() or not any(output.iterdir()):
        return {}
    manifest = output / MANIFEST
    if not manifest.is_file() or manifest.is_symlink():
        raise IconError("Nonempty output directory is not managed by this script; choose a separate Dev directory.")
    try:
        record = json.loads(manifest.read_text(encoding="utf-8"))
    except (OSError, ValueError) as exc:
        raise IconError("Output ownership manifest is unreadable; existing resources were kept.") from exc
    if not isinstance(record, dict) or record.get("tool") != TOOL or record.get("schema") != 1 or not isinstance(record.get("files"), dict):
        raise IconError("Output ownership manifest is not recognized.")
    files = record["files"]
    if not files:
        raise IconError("Output ownership manifest has no generated files.")
    for name, expected in files.items():
        if not isinstance(name, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,100}\.(png|ico|icns)", name):
            raise IconError("Invalid generated filename in ownership manifest.")
        path = output / name
        if not isinstance(expected, str) or not re.fullmatch(r"[0-9a-f]{64}", expected):
            raise IconError("Invalid generated checksum in ownership manifest.")
        if path.is_symlink() or (path.exists() and (not path.is_file() or digest(path.read_bytes()) != expected)):
            raise IconError(f"Generated resource was changed or replaced: {name}; existing resources were kept.")
    if not force:
        raise IconError("Dev resources already exist. Pass --force to regenerate verified script-owned outputs.")
    return files


def directory_snapshot(directory):
    """Detect changes to unrelated files before replacing a staged directory."""
    snapshot = {}
    for path in [directory, *sorted(directory.rglob("*"))]:
        status = path.lstat()
        key = path.relative_to(directory).as_posix()
        if stat.S_ISLNK(status.st_mode):
            content = str(path.readlink())
        elif stat.S_ISREG(status.st_mode):
            content = digest(path.read_bytes())
        elif stat.S_ISDIR(status.st_mode):
            content = None
        else:
            raise IconError(f"Output directory contains an unsupported file: {key}.")
        snapshot[key] = (status.st_mode, status.st_mtime_ns, content)
    return snapshot


def marked_png(data):
    # Standard PNG tEXt marker prevents accidentally badging a generated PNG again.
    payload = b"tEXt" + TOOL.encode() + b"\0" + b"1"
    chunk = struct.pack(">I", len(payload) - 4) + payload + struct.pack(">I", zlib.crc32(payload))
    return data[:33] + chunk + data[33:]


def write_resources(stage, source, args, formats, sizes, required):
    names = []

    def reserve(name):
        path = stage / name
        if path.exists() or path.is_symlink():
            raise IconError(f"New output would overwrite an unrelated file: {name}.")
        names.append(name)
        return path

    with tempfile.TemporaryDirectory(prefix=".render-", dir=stage.parent) as temporary:
        paths = {size: Path(temporary) / f"{size}.png" for size in sorted(required)}
        native_renderer(request={"source": str(source), "outputs": {str(size): str(path) for size, path in paths.items()}})
        rendered = {size: marked_png(path.read_bytes()) for size, path in paths.items()}
    if "png" in formats:
        for size in sizes:
            reserve(f"{args.name}-{size}.png").write_bytes(rendered[size])
        reserve(f"{args.name}.png").write_bytes(rendered[max(sizes)])
    if "ico" in formats:
        # Modern ICO stores PNG representations with a little-endian directory.
        offset = 6 + 16 * len(ICO_SIZES)
        entries = []
        for size in ICO_SIZES:
            data = rendered[size]
            entries.append(struct.pack("<BBBBHHII", size % 256, size % 256, 0, 0, 1, 32, len(data), offset))
            offset += len(data)
        reserve(f"{args.name}.ico").write_bytes(
            struct.pack("<HHH", 0, 1, len(entries)) + b"".join(entries) + b"".join(rendered[size] for size in ICO_SIZES))
    if "icns" in formats:
        # PNG-backed standard and Retina slots, matching Apple's iconset sizes.
        slots = ((b"icp4", 16), (b"ic11", 32), (b"icp5", 32), (b"ic12", 64),
                 (b"icp6", 64), (b"ic07", 128), (b"ic13", 256), (b"ic08", 256),
                 (b"ic14", 512), (b"ic09", 512), (b"ic10", 1024))
        chunks = b"".join(kind + struct.pack(">I", len(rendered[size]) + 8) + rendered[size] for kind, size in slots)
        reserve(f"{args.name}.icns").write_bytes(b"icns" + struct.pack(">I", len(chunks) + 8) + chunks)
    return {name: digest((stage / name).read_bytes()) for name in sorted(names)}


def generate(args):
    if args.output_dir.is_symlink():
        raise IconError("Output directory cannot be a symbolic link.")
    source = args.input.resolve(strict=True)
    output = args.output_dir.resolve()
    if not source.is_file():
        raise IconError("Input must be a regular image file.")
    if source.is_relative_to(output):
        raise IconError("Output directory must not contain the original input artwork.")
    frames, source_hash = read_source(source)
    formats, sizes, required = parse_options(args, max(frames))
    old_files = owned_outputs(output, args.force)
    original_snapshot = directory_snapshot(output) if output.exists() else None
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".dev-icon-", dir=output.parent) as temporary:
        stage = Path(temporary) / "resources"
        if output.exists():
            shutil.copytree(output, stage, symlinks=True)
            for name in old_files:
                (stage / name).unlink(missing_ok=True)
            (stage / MANIFEST).unlink(missing_ok=True)
        else:
            stage.mkdir()
        files = write_resources(stage, source, args, formats, sizes, required)
        record = {"schema": 1, "tool": TOOL, "source": source.name, "source_sha256": source_hash,
                  "generator_sha256": digest(Path(__file__).read_bytes() + Path(__file__).with_name("render-dev-icon.swift").read_bytes()),
                  "name": args.name, "formats": sorted(formats),
                  "png_sizes": sizes if "png" in formats else [], "files": files}
        (stage / MANIFEST).write_text(json.dumps(record, ensure_ascii=False, sort_keys=True, indent=2) + "\n", encoding="utf-8")
        # Recheck ownership after rendering; don't overwrite a concurrent manual edit.
        if output.exists():
            if owned_outputs(output, args.force) != old_files or directory_snapshot(output) != original_snapshot:
                raise IconError("Output changed during generation; existing resources were kept.")
            # Keep a failed recovery outside TemporaryDirectory's automatic cleanup.
            backup = Path(tempfile.mkdtemp(prefix=f".{output.name}-backup-", dir=output.parent))
            backup.rmdir()
            output.rename(backup)
            try:
                if directory_snapshot(backup) != original_snapshot:
                    raise IconError("Output changed during replacement; existing resources were kept.")
                stage.rename(output)
            except (OSError, IconError) as exc:
                try:
                    backup.rename(output)
                except OSError as recovery:
                    raise IconError(f"Replacement and recovery failed; previous resources are preserved at {backup}: {recovery}") from exc
                raise
            try:
                shutil.rmtree(backup)
            except OSError as exc:
                print(f"dev-icon: generated resources are ready; previous directory kept at {backup}: {exc}", file=sys.stderr)
        else:
            if original_snapshot is not None:
                raise IconError("Output was removed during generation; replacement was stopped.")
            stage.rename(output)
    return output, files


def main(argv=None):
    args = parser().parse_args(argv)
    try:
        output, files = generate(args)
    except (IconError, OSError, ValueError) as exc:
        print(f"dev-icon: {exc}", file=sys.stderr)
        return 1
    print(f"Generated {len(files)} Dev icon files in {output}")
    for name in files:
        print(f"  {name}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
