#!/bin/bash
set -euo pipefail
ARCH="${1:?usage: build-msix.sh arm64|amd64 VERSION}"
VERSION="${2:?usage: build-msix.sh ARCH VERSION}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TEMPLATES_DIR="$REPO_ROOT/templates/release"

# MSIX ProcessorArchitecture uses "x64" for amd64; arm64 passes through.
case "$ARCH" in
  amd64) MSIX_ARCH="x64" ;;
  arm64) MSIX_ARCH="arm64" ;;
  *) echo "ERROR: unsupported ARCH: $ARCH (want amd64 or arm64)" >&2; exit 1 ;;
esac

BINARY="$REPO_ROOT/.tmp/dist/windows-${ARCH}/spacewave.exe"
HELPER="$REPO_ROOT/dist/helper/windows-${ARCH}/spacewave-helper.exe"
ICON="$REPO_ROOT/.tmp/icons/icon-256.png"
OUT="$REPO_ROOT/dist/installers"
LAYOUT="$REPO_ROOT/.tmp/msix-layout-${ARCH}"
MSIX="$OUT/spacewave-windows-${ARCH}.msix"

rm -rf "$LAYOUT"
mkdir -p "$OUT" "$LAYOUT/Assets"

# Verify build artifacts exist.
if [ ! -f "$BINARY" ]; then
  echo "ERROR: missing entrypoint binary: $BINARY" >&2
  exit 1
fi

# 1. Copy entrypoint binary.
cp "$BINARY" "$LAYOUT/Spacewave.exe"

# 2. Copy helper binary.
if [ -f "$HELPER" ]; then
  cp "$HELPER" "$LAYOUT/spacewave-helper.exe"
fi

# 3. Copy icon assets.
for size in 48 128 256; do
  cp ".tmp/icons/icon-${size}.png" "$LAYOUT/Assets/Square${size}x${size}Logo.png"
done

# 4. Generate AppxManifest.xml with version + arch templated.
sed -e "s/{{VERSION}}/$VERSION.0/g" \
    -e "s/{{ARCH}}/$MSIX_ARCH/g" \
    "$TEMPLATES_DIR/AppxManifest.xml" > "$LAYOUT/AppxManifest.xml"

case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*)
    MAKEAPPX="$(powershell.exe -NoProfile -Command "[string](Get-ChildItem 'C:\\Program Files (x86)\\Windows Kits\\10\\bin' -Recurse -Filter makeappx.exe | Sort-Object FullName -Descending | Select-Object -First 1 -ExpandProperty FullName)" | tr -d '\r')"
    if [ -z "$MAKEAPPX" ]; then
      echo "ERROR: makeappx.exe not found in Windows SDK." >&2
      exit 1
    fi
    LAYOUT_WIN="$(cygpath -w "$LAYOUT")"
    MSIX_WIN="$(cygpath -w "$MSIX")"
    powershell.exe -NoProfile -Command "& '$MAKEAPPX' pack /d '$LAYOUT_WIN' /p '$MSIX_WIN' /o"
    ;;
  *)
    # 5. Pack MSIX inside the spacewave-builder container. makemsix is from
    #    microsoft/msix-packaging and ships as a Linux-only binary in our
    #    image, so packing runs there via the same docker create+cp pattern as
    #    build-appimage.sh. No bind mounts -> works on Lima / colima / Docker
    #    Desktop equivalently. az sign on step 6 still runs on the host
    #    because Azure Trusted Signing auth lives there.
    IMAGE="spacewave-builder"

    if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
      echo "=== building $IMAGE image ==="
      docker build --platform linux/amd64 -t "$IMAGE" "$REPO_ROOT/desktop/"
    fi

    echo "=== [msix/$ARCH] start container ==="
    CID="$(docker create --platform linux/amd64 --rm \
        "$IMAGE" tail -f /dev/null)"
    trap "docker rm -f '$CID' >/dev/null 2>&1 || true" EXIT
    docker start "$CID" >/dev/null

    echo "=== [msix/$ARCH] copy layout ==="
    docker exec "$CID" mkdir -p /work/layout
    docker cp "$LAYOUT/." "$CID":/work/layout/

    echo "=== [msix/$ARCH] pack ==="
    docker exec "$CID" makemsix pack -d /work/layout -p /work/out.msix

    echo "=== [msix/$ARCH] copy artifact ==="
    docker cp "$CID:/work/out.msix" "$MSIX"

    docker rm -f "$CID" >/dev/null
    trap - EXIT
    ;;
esac

# 6. Sign MSIX via Azure Trusted Signing.
# Shares the same env vars as bldr's Windows signing hook so one export covers
# both the inner .exe and the MSIX container. Unset profile = skip signing (for
# local iteration only; CI + release runs must set it).
SIGN_PROFILE="${BLDR_WINDOWS_SIGN_PROFILE:-}"
if [ -z "$SIGN_PROFILE" ]; then
  echo "WARN: BLDR_WINDOWS_SIGN_PROFILE unset; producing unsigned MSIX." >&2
  echo "Built: $MSIX (unsigned)"
  exit 0
fi
SIGN_ACCOUNT="${BLDR_WINDOWS_SIGN_ACCOUNT:-}"
if [ -z "$SIGN_ACCOUNT" ]; then
  echo "ERROR: BLDR_WINDOWS_SIGN_PROFILE is set but BLDR_WINDOWS_SIGN_ACCOUNT is not." >&2
  exit 1
fi
SIGN_PUBLISHER="${BLDR_WINDOWS_SIGN_PUBLISHER:-Aperture Robotics, LLC.}"

az sign \
  --file "$MSIX" \
  --publisher-name "$SIGN_PUBLISHER" \
  --description "Spacewave" \
  --trusted-signing-account "$SIGN_ACCOUNT" \
  --trusted-signing-cert-profile "$SIGN_PROFILE"

echo "Built and signed: $MSIX"
