#!/usr/bin/env bash
# Run the shared-daemon Electron fixtures on a disposable X display.
set -euo pipefail
cd "$(dirname "$0")/../.."

: "${SPACEWAVE_SHARED_DESKTOP_FIXTURE:?Set the prepared Electron app directory}"
: "${SPACEWAVE_SHARED_DESKTOP_ELECTRON:?Set the Electron executable path}"

runtime=$(mktemp -d)
mkfifo "$runtime/display"
Xvfb -displayfd 3 -screen 0 1280x800x24 3>"$runtime/display" >"$runtime/xvfb.log" 2>&1 &
xvfb=$!
trap 'kill "$xvfb" 2>/dev/null || true; wait "$xvfb" 2>/dev/null || true; rm -rf "$runtime"' EXIT
read -r display <"$runtime/display"
export DISPLAY=:$display

# Each test gets its own two-minute budget; report every failure.
status=0
for name in TestSharedDaemonElectron TestSharedDaemonElectronQuit TestSharedDaemonElectronTray TestSharedDaemonElectronRoots; do
  go test -p="${GO_TEST_JOBS:-2}" -timeout=110s ./cmd/spacewave/cli \
    -run "^${name}\$" -count=1 -v "$@" || status=1
done
exit "$status"
