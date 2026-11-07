package kvtx

import (
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// holdLeases stores the block R of tr, unless stored, and holds n leased
// roots named prefix0 to prefixN-1 on it, each expiring at expires.
func holdLeases(t *testing.T, v *Volume, tr testTree, prefix string, n int, expires time.Time) {
	t.Helper()
	ctx := t.Context()
	if tr["R"] == nil {
		tr.add(t, "R")
	}
	roots := make([]block.NamedRoot, n)
	for i := range roots {
		roots[i] = block.NamedRoot{Name: prefix + strconv.Itoa(i), Ref: tr["R"].Ref, Expires: expires}
	}
	err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
		_, err := v.land(ctx, p, "", tr.entries("R"), roots)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// collectLapsed runs one collection pass and returns the leases it released.
func collectLapsed(t *testing.T, v *Volume) uint64 {
	t.Helper()
	res, err := v.collector.Collect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return res.Lapsed
}

// TestLeaseSweepStartsAtItsMark checks that the lease sweep releases every
// lapsed lease across its slices, then starts its next pass at the expiry the
// last one reached, so the deleted index records below it are not read: a
// lease written below the mark waits for the hourly scan from the start, or
// for a clock that stepped back past the mark.
func TestLeaseSweepStartsAtItsMark(t *testing.T) {
	t.Run("crosses slices", func(t *testing.T) {
		// 600 lapsed leases take three slices of one pass.
		v, clk := newLivenessVolume(t, false)
		tr := testTree{}
		holdLeases(t, v, tr, "lease/", 600, clk.Now().Add(rootLease))
		clk.Add(rootLease + time.Second)
		if got := collectLapsed(t, v); got != 600 {
			t.Fatalf("one pass released %d leases, want 600", got)
		}
		if got := collectLapsed(t, v); got != 0 {
			t.Fatalf("the next pass released %d leases, want 0", got)
		}
	})

	t.Run("below the mark", func(t *testing.T) {
		// The first pass scans from the start and sets the mark at now.
		v, clk := newLivenessVolume(t, false)
		tr := testTree{}
		holdLeases(t, v, tr, "early/", 3, clk.Now().Add(-time.Minute))
		if got := collectLapsed(t, v); got != 3 {
			t.Fatalf("the first pass released %d leases, want 3", got)
		}

		// A lease written below the mark is not found from the mark, while
		// one written above it lapses on time.
		holdLeases(t, v, tr, "below/", 1, clk.Now().Add(-time.Minute))
		holdLeases(t, v, tr, "above/", 1, clk.Now().Add(time.Minute))
		clk.Add(2 * time.Minute)
		if got := collectLapsed(t, v); got != 1 {
			t.Fatalf("a pass from the mark released %d leases, want only the one above it", got)
		}

		// The scan from the start an hour after the last one finds it.
		clk.Add(time.Hour)
		if got := collectLapsed(t, v); got != 1 {
			t.Fatalf("the hourly scan released %d leases, want the one below the mark", got)
		}
	})

	t.Run("clock steps back", func(t *testing.T) {
		// A pass sets the mark, then the clock steps back ten minutes.
		v, clk := newLivenessVolume(t, false)
		tr := testTree{}
		if got := collectLapsed(t, v); got != 0 {
			t.Fatalf("an empty pass released %d leases", got)
		}
		clk.Add(-10 * time.Minute)

		// A lease that lapsed on the stepped back clock is found at once.
		holdLeases(t, v, tr, "back/", 1, clk.Now().Add(-time.Second))
		if got := collectLapsed(t, v); got != 1 {
			t.Fatalf("a pass after the clock stepped back released %d leases, want 1", got)
		}
	})
}
