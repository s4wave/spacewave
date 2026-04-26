#!/bin/bash
set -euo pipefail

PLATFORM="${1:?usage: build-helper.sh darwin|linux|windows}"
ARCH="${2:-amd64}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUT="$REPO_ROOT/dist/helper"
VERBOSE="${VERBOSE:-0}"
mkdir -p "$OUT"

# Keep build scratch directories outside the source tree under
# .tmp/<tool>/<platform> so they survive source-tree relocations and stay
# per-platform isolated. The in-source .build / build dirs are brittle:
# Swift bakes absolute paths into its PCH module cache, and CMake caches
# the source path too, so a repo move (e.g. native/macos -> desktop/macos)
# invalidates them. Using a stable out-of-tree path avoids that class of
# breakage entirely.
SCRATCH_ROOT="$REPO_ROOT/.tmp"

# build_via_docker runs the CMake + Ninja cross-build inside the
# spacewave-builder image via the docker cp pattern so it works on hosts
# without 9p / bind-mount support (e.g. Lima). Source tree is copied into
# a transient container, built there, and the artifact is copied back out.
#
# Phases are printed explicitly so the first-time Zig target-cache extraction
# (glibc / mingw-w64 stub unpack) shows up as its own step instead of hiding
# inside cmake's ABI probe. Under qemu emulation that extraction dominates
# wall time on the first run per target.
#
#   build_via_docker <label> <triple> <toolchain> <artifact-in> <artifact-out>
build_via_docker() {
  local label="$1"
  local triple="$2"
  local toolchain="$3"
  local artifact_in="$4"
  local artifact_out="$5"
  local image="spacewave-builder"

  echo "=== [$label] ensure image ==="
  if ! docker image inspect "$image" >/dev/null 2>&1; then
    docker build --platform linux/amd64 -t "$image" "$REPO_ROOT/desktop/"
  fi

  echo "=== [$label] start container ==="
  # Persistent cache for Zig's per-target stub extraction so only the first
  # build per target pays the cost; every subsequent build reuses
  # /root/.cache/zig. Defaults to a docker named volume for local iteration.
  # CI sets BLDR_BUILDER_ZIG_CACHE_DIR to a host path so GitHub Actions cache
  # can persist it across workflow runs.
  local zig_cache_mount="type=volume,source=spacewave-builder-zig-cache,target=/root/.cache/zig"
  if [ -n "${BLDR_BUILDER_ZIG_CACHE_DIR:-}" ]; then
    mkdir -p "$BLDR_BUILDER_ZIG_CACHE_DIR"
    zig_cache_mount="type=bind,source=$BLDR_BUILDER_ZIG_CACHE_DIR,target=/root/.cache/zig"
  fi
  local cid
  cid="$(docker create --platform linux/amd64 --rm \
      --mount "$zig_cache_mount" \
      "$image" tail -f /dev/null)"
  # Double-quoted trap expands $cid at setup so cleanup still works after the
  # function returns and the local goes out of scope.
  trap "docker rm -f '$cid' >/dev/null 2>&1 || true" EXIT
  docker start "$cid" >/dev/null

  echo "=== [$label] copy source ==="
  docker exec "$cid" mkdir -p /src /build /probe
  docker cp "$REPO_ROOT/desktop/cross/." "$cid":/src/

  echo "=== [$label] warm zig target cache ($triple) ==="
  # Compile AND link a trivial C++ program so Zig extracts libc, libc++, and
  # (on windows) mingw-w64 import libs for the target now, with a visible
  # phase marker. cmake's ABI probe does a full compile+link too, so doing it
  # here means cmake reuses the warm cache instead of hiding the extraction.
  local zig_verbose=""
  if [ "$VERBOSE" != "0" ]; then
    # zig cc / zig c++ forward -v to the underlying clang driver which prints
    # every sub-invocation (compile, assembler, linker) with full arg list.
    zig_verbose="-v"
  fi
  docker exec "$cid" sh -c 'printf "int main(){return 0;}\n" > /probe/probe.cc'
  docker exec "$cid" zig c++ --target="$triple" $zig_verbose /probe/probe.cc -o /probe/probe

  echo "=== [$label] cmake configure ==="
  local cmake_configure_extra=""
  if [ "$VERBOSE" != "0" ]; then
    cmake_configure_extra="-DCMAKE_VERBOSE_MAKEFILE=ON"
  fi
  docker exec "$cid" cmake -S /src -B /build \
      --toolchain "/src/toolchains/$toolchain" \
      -G Ninja -DCMAKE_BUILD_TYPE=Release $cmake_configure_extra

  echo "=== [$label] cmake build ==="
  local ninja_flags=""
  if [ "$VERBOSE" != "0" ]; then
    ninja_flags="-v"
  fi
  docker exec "$cid" cmake --build /build -- $ninja_flags

  echo "=== [$label] copy artifact ==="
  docker cp "$cid:$artifact_in" "$artifact_out"

  docker rm -f "$cid" >/dev/null
  trap - EXIT
}

# sign_windows_helper signs the extracted Windows helper .exe via Azure
# Trusted Signing. Mirrors the pattern in scripts/build-msix.sh so one env
# export covers the inner bldr-signed .exe, the MSIX container, and now the
# C++ helper .exe. Unset profile = warn and skip (local iteration only;
# release runs must set it).
sign_windows_helper() {
  local exe="$1"
  local profile="${BLDR_WINDOWS_SIGN_PROFILE:-}"
  if [ -z "$profile" ]; then
    echo "WARN: BLDR_WINDOWS_SIGN_PROFILE unset; Windows helper is unsigned." >&2
    return 0
  fi
  local account="${BLDR_WINDOWS_SIGN_ACCOUNT:-}"
  if [ -z "$account" ]; then
    echo "ERROR: BLDR_WINDOWS_SIGN_PROFILE is set but BLDR_WINDOWS_SIGN_ACCOUNT is not." >&2
    exit 1
  fi
  local publisher="${BLDR_WINDOWS_SIGN_PUBLISHER:-Aperture Robotics, LLC.}"

  echo "=== [sign] $exe ==="
  az sign \
    --file "$exe" \
    --publisher-name "$publisher" \
    --description "Spacewave Helper" \
    --trusted-signing-account "$account" \
    --trusted-signing-cert-profile "$profile"
}

# Per-platform output is namespaced under <goos>-<goarch>/ so darwin, linux,
# and windows helpers never collide at the same filename (previous layout
# wrote all three to dist/helper/spacewave-helper-<arch> which let the linux
# ELF overwrite the darwin Mach-O on multi-platform release runs, producing
# a .app bundle whose helper was an unlaunchable Linux binary).
case "$PLATFORM" in
  darwin)
    cd "$REPO_ROOT/desktop/macos"
    SCRATCH_ARM="$SCRATCH_ROOT/swift/darwin-arm64"
    SCRATCH_AMD="$SCRATCH_ROOT/swift/darwin-amd64"
    mkdir -p "$SCRATCH_ARM" "$SCRATCH_AMD"
    swift build -c release --arch arm64 --scratch-path "$SCRATCH_ARM"
    swift build -c release --arch x86_64 --scratch-path "$SCRATCH_AMD"
    mkdir -p "$OUT/darwin-arm64" "$OUT/darwin-amd64"
    cp "$SCRATCH_ARM/arm64-apple-macosx/release/SpacewaveHelper" \
      "$OUT/darwin-arm64/spacewave-helper"
    cp "$SCRATCH_AMD/x86_64-apple-macosx/release/SpacewaveHelper" \
      "$OUT/darwin-amd64/spacewave-helper"
    echo "Built macOS helpers in $OUT/darwin-{arm64,amd64}/"
    ;;
  linux)
    case "$ARCH" in
      amd64) TRIPLE="x86_64-linux-gnu" ;;
      arm64) TRIPLE="aarch64-linux-gnu" ;;
      *) echo "ERROR: unsupported linux helper ARCH: $ARCH" >&2; exit 1 ;;
    esac
    mkdir -p "$OUT/linux-${ARCH}"
    build_via_docker "linux/$ARCH" "$TRIPLE" "linux-${ARCH}.cmake" \
        "/build/spacewave-helper" \
        "$OUT/linux-${ARCH}/spacewave-helper"
    echo "Built Linux helper ($ARCH) in $OUT/linux-${ARCH}/"
    ;;
  windows)
    case "$ARCH" in
      amd64) TRIPLE="x86_64-windows-gnu" ;;
      arm64) TRIPLE="aarch64-windows-gnu" ;;
      *) echo "ERROR: unsupported windows helper ARCH: $ARCH" >&2; exit 1 ;;
    esac
    mkdir -p "$OUT/windows-${ARCH}"
    build_via_docker "windows/$ARCH" "$TRIPLE" "windows-${ARCH}.cmake" \
        "/build/spacewave-helper.exe" \
        "$OUT/windows-${ARCH}/spacewave-helper.exe"
    sign_windows_helper "$OUT/windows-${ARCH}/spacewave-helper.exe"
    echo "Built Windows helper ($ARCH) in $OUT/windows-${ARCH}/"
    ;;
  *)
    echo "error: unknown platform $PLATFORM"
    exit 1
    ;;
esac
