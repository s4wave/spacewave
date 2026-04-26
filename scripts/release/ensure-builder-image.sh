#!/bin/bash
set -euo pipefail

# ensure-builder-image.sh builds the spacewave-builder Docker image if it's
# not already present AND passes a fast smoke test. Pulling the build into
# its own script means release.go can call it once before the per-platform
# loop, so subsequent build-helper.sh invocations reuse the cached image
# instead of each racing to `docker build` it.
#
# Smoke test runs after `docker image inspect` and after a fresh build: it
# exercises each cross-compilation tool the image is supposed to provide
# (zig, appimagetool) so a stale image from a prior version of the
# Dockerfile surfaces as a clear failure here instead of deep into a
# release run. Set REBUILD_ON_SMOKE_FAIL=1 to auto-rebuild when the smoke
# check trips.

IMAGE="spacewave-builder"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

smoke_test() {
  # Each check must exit 0 for the image to be considered healthy. Probes
  # each tool the image provides so a stale image missing zig or
  # appimagetool surfaces as a clear failure here instead of deep into a
  # release run.
  #
  # makemsix probe removed 2026-04-20: Azure approval is done, but MSIX still
  # needs a Windows signing hop before the path can be re-enabled. Restore the
  # probe when the makemsix build stanza is re-enabled in desktop/Dockerfile.
  docker run --rm --platform linux/amd64 "$IMAGE" zig version >/dev/null
  docker run --rm --platform linux/amd64 \
      -e APPIMAGE_EXTRACT_AND_RUN=1 \
      "$IMAGE" appimagetool --version >/dev/null
}

rebuild_image() {
  echo "=== building $IMAGE image ==="
  # CI sets BLDR_BUILDER_USE_BUILDX_GHA=1 so the slow apt + zig + appimagetool
  # layers in desktop/Dockerfile are cached across workflow runs via the
  # GitHub Actions cache backend. --load is required because subsequent steps
  # (smoke test, build-helper.sh) shell out to plain `docker run` and need
  # the image present in the local docker engine.
  if [ "${BLDR_BUILDER_USE_BUILDX_GHA:-0}" = "1" ]; then
    docker buildx build --platform linux/amd64 \
        --cache-from=type=gha,scope=spacewave-builder \
        --cache-to=type=gha,mode=max,scope=spacewave-builder \
        --load \
        -t "$IMAGE" "$REPO_ROOT/desktop/"
    return
  fi
  docker build --platform linux/amd64 -t "$IMAGE" "$REPO_ROOT/desktop/"
}

if docker image inspect "$IMAGE" >/dev/null 2>&1; then
  echo "=== $IMAGE image present, running smoke test ==="
  if smoke_test; then
    echo "=== smoke test OK ==="
    exit 0
  fi
  echo "=== smoke test FAILED ==="
  if [ "${REBUILD_ON_SMOKE_FAIL:-0}" = "1" ]; then
    echo "=== rebuilding image (REBUILD_ON_SMOKE_FAIL=1) ==="
    docker rmi "$IMAGE" >/dev/null 2>&1 || true
    rebuild_image
    echo "=== post-rebuild smoke test ==="
    smoke_test
    echo "=== smoke test OK ==="
    exit 0
  fi
  echo "existing $IMAGE image is stale or missing tools." >&2
  echo "Fix: docker rmi $IMAGE  (or re-run with REBUILD_ON_SMOKE_FAIL=1)" >&2
  exit 1
fi

rebuild_image
echo "=== running smoke test on fresh image ==="
smoke_test
echo "=== smoke test OK ==="
