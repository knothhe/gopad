#!/bin/bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "App icon generation requires macOS (AppKit, sips, and iconutil)." >&2
  exit 1
fi

ICON_TMP="$(mktemp -d "${TMPDIR:-/tmp}/gopad-icon.XXXXXX")"
trap 'rm -rf "$ICON_TMP"' EXIT
ICONSET="$ICON_TMP/gopad.iconset"
mkdir -p "$ICONSET"

swift "$PROJECT_DIR/scripts/render-icon.swift" \
  "$PROJECT_DIR/assets/gopad.svg" "$PROJECT_DIR/assets/gopad.png"

for SIZE in 16 32 128 256 512; do
  # macOS 27 puts standard-resolution 16/32px ICNS artwork on a grey
  # legacy plate. Let IconServices downsample the Retina PNGs instead.
  if [[ "$SIZE" -ge 128 ]]; then
    sips -z "$SIZE" "$SIZE" "$PROJECT_DIR/assets/gopad.png" \
      --out "$ICONSET/icon_${SIZE}x${SIZE}.png" >/dev/null
  fi
  DOUBLE=$((SIZE * 2))
  sips -z "$DOUBLE" "$DOUBLE" "$PROJECT_DIR/assets/gopad.png" \
    --out "$ICONSET/icon_${SIZE}x${SIZE}@2x.png" >/dev/null
done

iconutil -c icns "$ICONSET" -o "$PROJECT_DIR/assets/gopad.icns"
echo "Generated assets/gopad.png and assets/gopad.icns from assets/gopad.svg"
