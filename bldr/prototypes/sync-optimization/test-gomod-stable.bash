#!/bin/bash
# test-gomod-stable.bash: Test whether go.mod/go.sum output is deterministic
# between consecutive SyncDistSources runs.
#
# Uses main .bldr/ and bldr-demo-cli manifest.
# Run from bldr repo root:
#   bash prototypes/sync-optimization/test-gomod-stable.bash
set -eo pipefail

BLDR_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SRC_DIR="$BLDR_ROOT/.bldr/src"
TMP_DIR="/tmp/sync-opt-test"

mkdir -p "$TMP_DIR"

if [ ! -f "$SRC_DIR/go.mod" ]; then
    echo "ERROR: .bldr/src/go.mod not found."
    echo "Run 'bun run bldr -- start cli bldr-demo-cli -- status' first."
    exit 1
fi

echo "=== go.mod/go.sum Stability Test ==="
echo ""

cd "$BLDR_ROOT"

# Run 1
echo "--- Run 1: bldr start cli ---"
bun run go:run -- \
    github.com/s4wave/spacewave/bldr/cmd/bldr \
    --bldr-src-path=../../ \
    start cli bldr-demo-cli -- status 2>&1 | grep -E "INFO" | tail -5

cp "$SRC_DIR/go.mod" "$TMP_DIR/go.mod.run1"
cp "$SRC_DIR/go.sum" "$TMP_DIR/go.sum.run1"
md5 -q "$SRC_DIR/vendor/modules.txt" > "$TMP_DIR/vendor.md5.run1" 2>/dev/null || echo "none" > "$TMP_DIR/vendor.md5.run1"
echo "  Snapshot saved (run 1)"
echo "  go.mod: $(wc -c < "$TMP_DIR/go.mod.run1") bytes, $(wc -l < "$TMP_DIR/go.mod.run1") lines"
echo "  go.sum: $(wc -c < "$TMP_DIR/go.sum.run1") bytes"
echo ""

# Run 2
echo "--- Run 2: bldr start cli (should produce identical state) ---"
bun run go:run -- \
    github.com/s4wave/spacewave/bldr/cmd/bldr \
    --bldr-src-path=../../ \
    start cli bldr-demo-cli -- status 2>&1 | grep -E "INFO" | tail -5

cp "$SRC_DIR/go.mod" "$TMP_DIR/go.mod.run2"
cp "$SRC_DIR/go.sum" "$TMP_DIR/go.sum.run2"
md5 -q "$SRC_DIR/vendor/modules.txt" > "$TMP_DIR/vendor.md5.run2" 2>/dev/null || echo "none" > "$TMP_DIR/vendor.md5.run2"
echo "  Snapshot saved (run 2)"
echo "  go.mod: $(wc -c < "$TMP_DIR/go.mod.run2") bytes, $(wc -l < "$TMP_DIR/go.mod.run2") lines"
echo "  go.sum: $(wc -c < "$TMP_DIR/go.sum.run2") bytes"
echo ""

# Compare
echo "=== Comparison ==="

if diff -q "$TMP_DIR/go.mod.run1" "$TMP_DIR/go.mod.run2" > /dev/null 2>&1; then
    echo "go.mod:             IDENTICAL"
else
    echo "go.mod:             CHANGED"
    diff "$TMP_DIR/go.mod.run1" "$TMP_DIR/go.mod.run2" | head -20 || true
fi

if diff -q "$TMP_DIR/go.sum.run1" "$TMP_DIR/go.sum.run2" > /dev/null 2>&1; then
    echo "go.sum:             IDENTICAL"
else
    echo "go.sum:             CHANGED"
    wc -l "$TMP_DIR/go.sum.run1" "$TMP_DIR/go.sum.run2"
    diff "$TMP_DIR/go.sum.run1" "$TMP_DIR/go.sum.run2" | head -20 || true
fi

if diff -q "$TMP_DIR/vendor.md5.run1" "$TMP_DIR/vendor.md5.run2" > /dev/null 2>&1; then
    echo "vendor/modules.txt: IDENTICAL"
else
    echo "vendor/modules.txt: CHANGED"
fi

echo ""
echo "=== Conclusion ==="
gomod_stable=true
gosum_stable=true

if ! diff -q "$TMP_DIR/go.mod.run1" "$TMP_DIR/go.mod.run2" > /dev/null 2>&1; then
    gomod_stable=false
fi
if ! diff -q "$TMP_DIR/go.sum.run1" "$TMP_DIR/go.sum.run2" > /dev/null 2>&1; then
    gosum_stable=false
fi

if $gomod_stable && $gosum_stable; then
    echo "PASS: go.mod and go.sum are deterministic between runs."
    echo ""
    echo "Optimization: after generating the modified go.mod in memory,"
    echo "compare with existing .bldr/src/go.mod. If bytes.Equal AND"
    echo "vendor/ exists: skip write + go mod tidy + go mod vendor."
    echo "Expected savings: ~1-2s per startup."
elif $gomod_stable; then
    echo "PARTIAL: go.mod is stable but go.sum differs."
    echo "Optimization: skip go mod tidy (go.sum is appended each run)."
    echo "May need to also hash go.sum for full skip."
else
    echo "FAIL: Output differs between runs."
    echo "Cannot use simple comparison. Need hash-based caching."
fi
