#!/bin/bash
# test-build-cache.bash: Test whether Go's build cache is effective
# for CLI manifest builds after the RemoveAll fix.
#
# The hypothesis: now that we don't destroy .bldr/src/ on every run,
# Go's build cache should make the 2nd go build nearly instant.
#
# Run from bldr repo root:
#   bash prototypes/sync-optimization/test-build-cache.bash
set -eo pipefail

BLDR_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
PROTO_DIR="$BLDR_ROOT/prototypes/sync-optimization"
STATE_DIR="$PROTO_DIR/.bldr"

echo "=== Go Build Cache Effectiveness Test ==="
echo ""

# We need a bldr.yaml for this prototype. Use simple's if ours doesn't exist.
BLDR_YAML="$PROTO_DIR/bldr.yaml"
if [ ! -f "$BLDR_YAML" ]; then
    echo "ERROR: Need $BLDR_YAML with a CLI manifest defined."
    echo "Create one or copy from prototypes/simple/bldr.yaml"
    exit 1
fi

# Clean state for a fair test
rm -rf "$STATE_DIR"

echo "--- Run 1: Cold start (no cache) ---"
t0=$(python3 -c 'import time; print(time.time())')

cd "$BLDR_ROOT"
bun run go:run -- \
    github.com/s4wave/spacewave/bldr/cmd/bldr \
    --bldr-src-path=../../ \
    --state-path "$STATE_DIR" \
    --log-level=info \
    -c "$BLDR_YAML" \
    setup 2>&1 | grep -E "INFO|building|committing|done" || true

t1=$(python3 -c 'import time; print(time.time())')
echo "Run 1 (cold): $(python3 -c "print(f'{$t1 - $t0:.3f}s')")"
echo ""

echo "--- Run 2: Warm start (cache should help) ---"
t2=$(python3 -c 'import time; print(time.time())')

bun run go:run -- \
    github.com/s4wave/spacewave/bldr/cmd/bldr \
    --bldr-src-path=../../ \
    --state-path "$STATE_DIR" \
    --log-level=info \
    -c "$BLDR_YAML" \
    setup 2>&1 | grep -E "INFO|building|committing|done" || true

t3=$(python3 -c 'import time; print(time.time())')
echo "Run 2 (warm): $(python3 -c "print(f'{$t3 - $t2:.3f}s')")"
echo ""

echo "--- Run 3: Warm start again (verify consistency) ---"
t4=$(python3 -c 'import time; print(time.time())')

bun run go:run -- \
    github.com/s4wave/spacewave/bldr/cmd/bldr \
    --bldr-src-path=../../ \
    --state-path "$STATE_DIR" \
    --log-level=info \
    -c "$BLDR_YAML" \
    setup 2>&1 | grep -E "INFO|building|committing|done" || true

t5=$(python3 -c 'import time; print(time.time())')
echo "Run 3 (warm): $(python3 -c "print(f'{$t5 - $t4:.3f}s')")"
echo ""

echo "=== Summary ==="
echo "Run 1 (cold):  $(python3 -c "print(f'{$t1 - $t0:.3f}s')")"
echo "Run 2 (warm):  $(python3 -c "print(f'{$t3 - $t2:.3f}s')")"
echo "Run 3 (warm):  $(python3 -c "print(f'{$t5 - $t4:.3f}s')")"
echo ""
echo "If Run 2/3 are significantly faster than Run 1, Go's build cache"
echo "is now working after the RemoveAll fix."
echo "If all runs are similar, the bottleneck is elsewhere (go mod tidy/vendor)."
