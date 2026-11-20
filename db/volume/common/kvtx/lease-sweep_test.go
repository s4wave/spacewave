package kvtx

import (
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// holdLeases stores the block R of tr, unless stored, and holds n roots named
// prefix0 to prefixN-1 on it under a new lease expiring at expires, whose ID
// it returns.
func holdLeases(t *testing.T, v *Volume, tr testTree, prefix string, n int, expires time.Time) string {
	t.Helper()
	ctx := t.Context()
	if tr["R"] == nil {
		tr.add(t, "R")
	}
	lease := ulid.NewULID()
	roots := make([]block.NamedRoot, n)
	for i := range roots {
		roots[i] = block.NamedRoot{Name: prefix + strconv.Itoa(i), Ref: tr["R"].Ref, Lease: lease}
	}
	err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
		if err := p.rec.StartLease(ctx, lease, expires.UnixMilli()); err != nil {
			return err
		}
		_, err := v.land(ctx, p, "", tr.entries("R"), roots)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return lease
}

// collectLapsed runs one collection pass and returns the leased roots it
// released.
func collectLapsed(t *testing.T, v *Volume) uint64 {
	t.Helper()
	res, err := v.collector.Collect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return res.Lapsed
}

// TestLeaseSweepReleasesLapsedLeases checks that one pass releases every root
// of a lapsed lease across its slices and ends the lease, and keeps a live
// lease and its root.
func TestLeaseSweepReleasesLapsedLeases(t *testing.T) {
	// 600 roots of a lapsed lease take three slices of one pass.
	v, clk := newLivenessVolume(t, false)
	ctx := t.Context()
	tr := testTree{}
	lapsed := holdLeases(t, v, tr, "lease/", 600, clk.Now().Add(rootLease))
	live := holdLeases(t, v, tr, "live/", 1, clk.Now().Add(2*rootLease))
	clk.Add(rootLease + time.Second)
	if got := collectLapsed(t, v); got != 600 {
		t.Fatalf("one pass released %d roots, want 600", got)
	}
	if got := collectLapsed(t, v); got != 0 {
		t.Fatalf("the next pass released %d roots, want 0", got)
	}

	// The lapsed lease ended, and the live one holds its root.
	err := v.readRecords(ctx, func(rec *block_gc.Records) error {
		if _, found, err := rec.GetLease(ctx, lapsed); err != nil || found {
			t.Fatalf("lapsed lease found %v err %v, want ended", found, err)
		}
		if expires, _, err := rec.GetLease(ctx, live); err != nil || expires <= clk.Now().UnixMilli() {
			t.Fatalf("live lease expires %d err %v, want live", expires, err)
		}
		entry, err := rec.GetRoot(ctx, "", "live/0")
		if err != nil || entry.GetLease() != live {
			t.Fatalf("live root %v err %v, want held under its lease", entry, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
