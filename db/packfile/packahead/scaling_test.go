package packahead

import (
	"testing"
)

// TestPackAheadScaling holds the allocations of Get, Append and Remove flat
// between n and 10n local packs, as the path walk's cost sentences require.
// The slack covers amortized table growth and the maintainer's finalizing,
// which run beside the measured calls; a cost per pack would add dozens.
func TestPackAheadScaling(t *testing.T) {
	small := measureCalls(t, 4)
	large := measureCalls(t, 40)
	t.Logf("allocations per call at 4 and 40 packs: %v, %v", small, large)
	for call, n := range small {
		if large[call] > n*1.2+2 {
			t.Errorf("%s: %.1f allocations at 4 packs, %.1f at 40", call, n, large[call])
		}
	}
}

// measureCalls fills a Log with packs finalized packs of 8 blocks and returns
// the allocations per call of each entry point.
func measureCalls(t *testing.T, packs int) map[string]float64 {
	ctx := t.Context()
	l, err := Open(ctx, t.TempDir(), Options{MaxPackBlocks: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// Fill the packs and wait for them to finalize.
	base := packs * 1000
	for i := range packs {
		var batch []Block
		for j := range 8 {
			batch = append(batch, testBlock(t, base+i*8+j, 3300))
		}
		if err := l.Append(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	waitFinalized(t, l, packs-1)
	old := testBlock(t, base, 3300)

	// Measure each entry point on the filled Log.
	next := base + packs*8
	fresh := func() Block {
		next++
		return testBlock(t, next, 3300)
	}
	pending := make([]Block, 0, 64)
	for range 64 {
		pending = append(pending, fresh())
	}
	i := 0
	return map[string]float64{
		"Get": testing.AllocsPerRun(50, func() {
			if _, ok, err := l.Get(old.Key); err != nil || !ok {
				t.Fatal("get missed", err)
			}
		}),
		"Append": testing.AllocsPerRun(20, func() {
			if err := l.Append(ctx, pending[i:i+1]); err != nil {
				t.Fatal(err)
			}
			i++
		}),
		"Remove": testing.AllocsPerRun(20, func() {
			l.Remove(pending[i%len(pending)].Key)
		}),
	}
}
