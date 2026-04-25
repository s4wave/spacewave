#!/bin/bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SOURCE="${1:-$REPO_ROOT/web/images/spacewave-icon.png}"
OUT="$REPO_ROOT/.tmp/icons"
mkdir -p "$OUT/icon.iconset"

for size in 16 32 64 128 256 512; do
  sips -z $size $size "$SOURCE" --out "$OUT/icon.iconset/icon_${size}x${size}.png" >/dev/null
  s2=$((size*2))
  sips -z $s2 $s2 "$SOURCE" --out "$OUT/icon.iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$OUT/icon.iconset" -o "$OUT/icon.icns"

for size in 16 32 48 64 128 256; do
  sips -z $size $size "$SOURCE" --out "$OUT/icon-${size}.png" >/dev/null
done
bun x png-to-ico "$OUT"/icon-{16,32,48,64,128,256}.png > "$OUT/icon.ico"

for size in 16 32 48 64 128 256 512; do
  sips -z $size $size "$SOURCE" --out "$OUT/icon-${size}.png" >/dev/null
done

rm -rf "$OUT/icon.iconset"
echo "Icons generated in $OUT"
