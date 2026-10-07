package s4db

import (
	"slices"
	"testing"
)

// TestSpaceDrainSkipsRetaken checks that drain returns only the released
// pages that are still free, so a page taken again is never punched.
func TestSpaceDrainSkipsRetaken(t *testing.T) {
	// Allocate pages, then free a run in the middle.
	sp := newSpace()
	sp.alloc(32)
	start := uint64(firstPage + 8)
	sp.addFree(run{start: start, n: 8})

	// Take the middle of the freed run again.
	sp.take(run{start: start + 2, n: 3})

	// Only the parts on either side may be punched.
	got := sp.drain()
	slices.SortFunc(got, func(a, b run) int { return int(a.start) - int(b.start) })
	want := []run{{start: start, n: 2}, {start: start + 5, n: 3}}
	if !slices.Equal(got, want) {
		t.Fatalf("drain = %v, want %v", got, want)
	}
	if rest := sp.drain(); len(rest) != 0 {
		t.Fatalf("second drain = %v, want none", rest)
	}
}
