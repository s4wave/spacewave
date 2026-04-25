#!/bin/bash
set -euo pipefail
ARCH="${1:?usage: build-winzip.sh arm64|amd64 VERSION}"
VERSION="${2:?usage: build-winzip.sh ARCH VERSION}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

case "$ARCH" in
  amd64|arm64) ;;
  *) echo "ERROR: unsupported ARCH: $ARCH (want amd64 or arm64)" >&2; exit 1 ;;
esac

BINARY="$REPO_ROOT/.tmp/dist/windows-${ARCH}/spacewave.exe"
HELPER="$REPO_ROOT/dist/helper/windows-${ARCH}/spacewave-helper.exe"
OUT="$REPO_ROOT/dist/installers"
LAYOUT="$REPO_ROOT/.tmp/winzip-layout-${ARCH}"
ZIP="$OUT/spacewave-windows-${ARCH}.zip"

mkdir -p "$OUT"

if [ ! -f "$BINARY" ]; then
  echo "ERROR: missing entrypoint binary: $BINARY" >&2
  exit 1
fi

rm -rf "$LAYOUT"
mkdir -p "$LAYOUT"

# Layout mirrors the MSIX root so the same launcher/helper paths work when a
# user extracts the zip and runs Spacewave.exe directly.
cp "$BINARY" "$LAYOUT/Spacewave.exe"
if [ -f "$HELPER" ]; then
  cp "$HELPER" "$LAYOUT/spacewave-helper.exe"
fi

# The inner .exe may already be signed before this script runs, either by
# bldr's gocompiler hook or by a later Windows-hosted signing step. We
# intentionally do not sign the outer zip: Authenticode does not define a
# container signature for plain zip, and SmartScreen evaluates the extracted
# .exe directly.
rm -f "$ZIP"
if command -v zip >/dev/null 2>&1; then
  (
    cd "$LAYOUT"
    zip -r -q "$OLDPWD/$ZIP" .
  )
else
  if ! command -v powershell.exe >/dev/null 2>&1; then
    echo "ERROR: neither zip nor powershell.exe is available for ZIP packaging." >&2
    exit 1
  fi
  LAYOUT_WIN="$(cygpath -w "$LAYOUT")"
  ZIP_WIN="$(cygpath -w "$ZIP")"
  powershell.exe -NoProfile -Command "Compress-Archive -Path '$LAYOUT_WIN\\*' -DestinationPath '$ZIP_WIN' -Force"
fi

echo "Built: $ZIP (inner .exe preserved as packaged)"
