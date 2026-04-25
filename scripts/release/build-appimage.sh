#!/bin/bash
set -euo pipefail
ARCH="${1:-amd64}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

case "$ARCH" in
  amd64|arm64) ;;
  *) echo "ERROR: unsupported ARCH: $ARCH (want amd64 or arm64)" >&2; exit 1 ;;
esac

BINARY="$REPO_ROOT/.tmp/dist/linux-${ARCH}/spacewave"
HELPER="$REPO_ROOT/dist/helper/linux-${ARCH}/spacewave-helper"
KVFILE="$REPO_ROOT/.tmp/plugins/linux-${ARCH}/plugin.kvfile"
ICON="$REPO_ROOT/.tmp/icons/icon-256.png"
OUT="$REPO_ROOT/dist/installers"
APPDIR="$REPO_ROOT/.tmp/Spacewave.AppDir-${ARCH}"

if [ ! -f "$BINARY" ]; then
  echo "ERROR: missing entrypoint binary: $BINARY" >&2
  exit 1
fi

rm -rf "$APPDIR"
mkdir -p "$OUT" "$APPDIR/usr/bin"

# 1. Copy entrypoint binary.
cp "$BINARY" "$APPDIR/usr/bin/spacewave"

# 2. Copy helper binary.
if [ -f "$HELPER" ]; then
  cp "$HELPER" "$APPDIR/usr/bin/spacewave-helper"
fi

# 3. Copy plugin kvfile.
if [ -f "$KVFILE" ]; then
  mkdir -p "$APPDIR/usr/share/spacewave"
  cp "$KVFILE" "$APPDIR/usr/share/spacewave/plugin.kvfile"
fi

# 4. Copy icon and desktop file.
cp "$ICON" "$APPDIR/spacewave.png"
cp "$REPO_ROOT/.tmp/spacewave.desktop" "$APPDIR/"

# 5. Create AppRun.
cat > "$APPDIR/AppRun" <<'RUN'
#!/bin/bash
HERE="$(dirname "$(readlink -f "$0")")"
exec "$HERE/usr/bin/spacewave" "$@"
RUN
chmod +x "$APPDIR/AppRun"

# 6. Pack via spacewave-builder container. appimagetool is a Linux binary
#    not available on darwin / windows hosts, so we run it inside the same
#    image used for helper cross-compilation (docker cp pattern, no bind
#    mounts -- works on Lima / colima / Docker Desktop the same way).
APPIMAGE="$OUT/spacewave-linux-${ARCH}.AppImage"
IMAGE="spacewave-builder"

# appimagetool's --runtime-arch flag takes the GOARCH-style suffix that matches
# the embedded runtime filename shipped inside appimagetool. It accepts
# x86_64/aarch64/armhf/i686; we pass the target arch via ARCH env.
case "$ARCH" in
  amd64) APPIMAGE_ARCH="x86_64" ;;
  arm64) APPIMAGE_ARCH="aarch64" ;;
esac

if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
  echo "=== building $IMAGE image ==="
  docker build --platform linux/amd64 -t "$IMAGE" "$REPO_ROOT/desktop/"
fi

echo "=== [appimage/$ARCH] start container ==="
CID="$(docker create --platform linux/amd64 --rm \
    -e ARCH="$APPIMAGE_ARCH" \
    -e APPIMAGE_EXTRACT_AND_RUN=1 \
    "$IMAGE" tail -f /dev/null)"
trap "docker rm -f '$CID' >/dev/null 2>&1 || true" EXIT
docker start "$CID" >/dev/null

echo "=== [appimage/$ARCH] copy AppDir ==="
docker exec "$CID" mkdir -p /work
docker cp "$APPDIR/." "$CID":/work/AppDir/

echo "=== [appimage/$ARCH] pack ==="
docker exec "$CID" appimagetool /work/AppDir /work/out.AppImage

echo "=== [appimage/$ARCH] copy artifact ==="
docker cp "$CID:/work/out.AppImage" "$APPIMAGE"

docker rm -f "$CID" >/dev/null
trap - EXIT

echo "Built: $APPIMAGE"
