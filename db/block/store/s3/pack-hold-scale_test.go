//go:build !tinygo

package block_store_s3

import (
	"fmt"
	"math"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// holdCost is the cost of one call of each read path: its allocations and its
// bucket reads.
type holdCost struct {
	// name names the read path.
	name string
	// allocs is the call's allocations.
	allocs float64
	// reads is the call's bucket reads.
	reads float64
}

// holdRuns is the calls each read path is measured over. The fallback lifts a
// pack each call, so each size holds more packs than this.
const holdRuns = 20

// holdRounds is the rounds each read path that lifts nothing is measured
// over, keeping the least: the allocations count the whole process, so a
// round may count an allocation of background work.
const holdRounds = 3

// measureHolds builds a store of one registered pack and held quarantined
// packs, and measures the cost of each read path over it.
func measureHolds(t *testing.T, kind string, held int) []holdCost {
	// Write the registered pack and the packs to hold, then quarantine them.
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	objects := &countingObjects{ObjectStore: client}
	s := openSweepStore(objects, newIndexKV(t, kind))
	defer s.Close()
	live := adoptBlocks(ctx, t, s, "live", 8)
	ids := make([]string, held)
	firsts := make([]*block.BlockRef, held)
	for i := range held {
		refs := adoptBlocks(ctx, t, s, fmt.Sprintf("held %d", i), 2)
		ids[i], firsts[i] = packOf(t, s, refs[0]), refs[0]
	}
	if err := s.Quarantine(ctx, ids); err != nil {
		t.Fatal(err)
	}

	// Check each path's answer once, which also warms the readers.
	absent, err := block.BuildBlockRef([]byte("absent"), nil)
	if err != nil {
		t.Fatal(err)
	}
	batch := append(append([]*block.BlockRef(nil), live...), firsts[held-8:]...)
	if s.readExtent(ctx, live[0]) == nil {
		t.Fatal("the extent read missed a registered block")
	}
	if found, err := s.LocateBlocks(ctx, batch); err != nil || found[0] != true || found[len(found)-1] != false {
		t.Fatalf("locate = %v, %v", found, err)
	}
	if _, found, err := s.GetBlock(ctx, absent); err != nil || found {
		t.Fatalf("read an absent block: %v, %v", found, err)
	}

	// Measure each path; the fallback reads the next held pack each call.
	next := 0
	paths := []struct {
		name  string
		fn    func()
		lifts bool
	}{
		{"extent read", func() { _ = s.readExtent(ctx, live[0]) }, false},
		{"registered read", func() { _, _, _ = s.GetBlock(ctx, live[1]) }, false},
		{"locate", func() { _, _ = s.LocateBlocks(ctx, batch) }, false},
		{"absent read", func() { _, _, _ = s.GetBlock(ctx, absent) }, false},
		{"fallback read", func() {
			if _, found, err := s.GetBlock(ctx, firsts[next]); err != nil || !found {
				t.Errorf("fallback read of held pack %d = %v, %v", next, found, err)
			}
			next++
		}, true},
	}
	costs := make([]holdCost, len(paths))
	for i, p := range paths {
		rounds := holdRounds
		if p.lifts {
			rounds = 1
		}
		costs[i] = holdCost{name: p.name, allocs: math.Inf(1), reads: math.Inf(1)}
		for range rounds {
			before := objects.entryReads.Load() + objects.rangeReads.Load()
			allocs := testing.AllocsPerRun(holdRuns, p.fn)
			reads := float64(objects.entryReads.Load()+objects.rangeReads.Load()-before) / (holdRuns + 1)
			costs[i].allocs, costs[i].reads = min(costs[i].allocs, allocs), min(costs[i].reads, reads)
		}
	}
	return costs
}

// TestHoldReadsScale holds the extent read, the read of a registered block,
// the locate, the read of an absent block and the fallback read flat in
// allocations and bucket reads between n and 10n quarantined packs, over each
// index volume.
func TestHoldReadsScale(t *testing.T) {
	if testing.Short() {
		t.Skip("writes hundreds of packs")
	}
	eachIndexVolume(t, func(t *testing.T, kind string) {
		const n = 40
		small := measureHolds(t, kind, n)
		large := measureHolds(t, kind, 10*n)
		for i, s := range small {
			l := large[i]
			t.Logf("%s: %d held: %.1f allocs, %.2f reads; %d held: %.1f allocs, %.2f reads", s.name, n, s.allocs, s.reads, 10*n, l.allocs, l.reads)
			// A B-tree a level deeper may allocate a node more, and where a
			// pack's tail lies varies the fallback's reads by a fraction of one,
			// with the allocations of each read, so a path that reads compares
			// its allocations per read; nothing grows with the held packs.
			sa, la := s.allocs, l.allocs
			if s.reads > 0 && l.reads > 0 {
				sa, la = s.allocs/s.reads, l.allocs/l.reads
			}
			if la > sa*1.1+2 || l.reads > s.reads+0.5 {
				t.Errorf("%s grew from %.1f allocs and %.2f reads to %.1f and %.2f", s.name, s.allocs, s.reads, l.allocs, l.reads)
			}
		}
	})
}
