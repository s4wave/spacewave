#!/bin/bash
# measure-phases.bash: Time each phase of SyncDistSources individually.
#
# Uses the existing .bldr/src/ directory from previous bldr runs.
# Run from bldr repo root:
#   bash prototypes/sync-optimization/measure-phases.bash
set -eo pipefail

BLDR_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SRC_DIR="$BLDR_ROOT/.bldr/src"

if [ ! -f "$SRC_DIR/go.mod" ]; then
    echo "ERROR: .bldr/src/go.mod not found."
    echo "Run 'bun run bldr -- start cli bldr-demo-cli -- status' first."
    exit 1
fi

time_ms() {
    python3 -c 'import time; print(time.time())'
}
elapsed() {
    python3 -c "print(f'{$2 - $1:.3f}s')"
}

echo "=== SyncDistSources Phase Timing ==="
echo "src dir: $SRC_DIR"
echo ""

# Save state for restoration
cp "$SRC_DIR/go.mod" /tmp/gomod-backup
cp "$SRC_DIR/go.sum" /tmp/gosum-backup

cd "$SRC_DIR"

# Phase 1: go mod tidy (1st run)
echo "--- Phase 1: go mod tidy (1st run) ---"
t0=$(time_ms)
go mod tidy 2>&1 | tail -3
t1=$(time_ms)
echo "  Time: $(elapsed "$t0" "$t1")"

# Phase 2: go mod vendor (1st run)
echo "--- Phase 2: go mod vendor (1st run) ---"
t2=$(time_ms)
go mod vendor 2>&1 | tail -3
t3=$(time_ms)
echo "  Time: $(elapsed "$t2" "$t3")"

# Phase 3: go mod tidy (2nd run -- should be no-op)
echo "--- Phase 3: go mod tidy (2nd run, no-op?) ---"
t4=$(time_ms)
go mod tidy 2>&1 | tail -3
t5=$(time_ms)
echo "  Time: $(elapsed "$t4" "$t5")"

# Phase 4: go mod vendor (2nd run -- should be no-op)
echo "--- Phase 4: go mod vendor (2nd run, no-op?) ---"
t6=$(time_ms)
go mod vendor 2>&1 | tail -3
t7=$(time_ms)
echo "  Time: $(elapsed "$t6" "$t7")"

echo ""

# Phase 5: Full bldr start cli (end-to-end, uses existing .bldr/)
echo "--- Phase 5: Full bldr start cli (end-to-end) ---"
cd "$BLDR_ROOT"

# Restore go.mod/go.sum to original state so SyncDistSources has work to do
cp /tmp/gomod-backup "$SRC_DIR/go.mod"
cp /tmp/gosum-backup "$SRC_DIR/go.sum"

t8=$(time_ms)
bun run go:run -- \
    github.com/s4wave/spacewave/bldr/cmd/bldr \
    --bldr-src-path=../../ \
    start cli bldr-demo-cli -- status 2>&1 | grep -E "INFO|done" | tail -10
t9=$(time_ms)
echo "  Time: $(elapsed "$t8" "$t9")"

echo ""
echo "=== Summary ==="
echo "go mod tidy  (1st): $(elapsed "$t0" "$t1")"
echo "go mod vendor (1st): $(elapsed "$t2" "$t3")"
echo "go mod tidy  (2nd): $(elapsed "$t4" "$t5")"
echo "go mod vendor (2nd): $(elapsed "$t6" "$t7")"
echo "full bldr start cli: $(elapsed "$t8" "$t9")"
echo ""
echo "Key insight: if 2nd-run times match 1st-run, go mod tidy/vendor"
echo "always scan the full module graph even as no-ops."
