package world_block_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// rehearsalLinkPred is the predicate the measured operation links with.
const rehearsalLinkPred = "<bench/rehearsal/link>"

// TestRehearsalChangesNothing checks that a rehearsal takes no writer slot,
// writes no block, leaves the head alone and refuses to commit, and that it
// sees the writes it applies.
func TestRehearsalChangesNothing(t *testing.T) {
	// Seed a small World and count the blocks written to its store.
	ctx := t.Context()
	tbed, rootRef, err := seedWorld(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}
	defer tbed.Release()
	eng, cleanup := openSeededEngine(ctx, t, tbed, rootRef)
	defer cleanup()
	var puts atomic.Int64
	world_block.WrapBlockStore(eng, world_block.ReadThrough{
		Get: func(ctx context.Context, store block.StoreOps, ref *block.BlockRef) ([]byte, bool, error) {
			return store.GetBlock(ctx, ref)
		},
		Put: func(*block.BlockRef) { puts.Add(1) },
	})

	// Open a rehearsal and apply an operation to it.
	rtx, err := eng.NewRehearsalTransaction(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	if err := applyRehearsalOp(ctx, rtx, 0, 200); err != nil {
		t.Fatal(err)
	}
	if !hasRehearsalLink(ctx, t, rtx, 0, 200) {
		t.Fatal("rehearsal does not see its own write")
	}

	// A write commits while the rehearsal is open.
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := seedWorldTx(wctx, eng, func(ws world.WorldState) error {
		return applyRehearsalOp(wctx, ws, 1, 200)
	}); err != nil {
		t.Fatalf("write while rehearsing: %v", err)
	}

	// Commit refuses without writing a block or moving the head.
	head := eng.GetRootRef().GetRootRef().Clone()
	before := puts.Load()
	if err := rtx.Commit(ctx); !errors.Is(err, world.ErrRehearsalCommit) {
		t.Fatalf("rehearsal commit returned %v", err)
	}
	if got := puts.Load() - before; got != 0 {
		t.Fatalf("rehearsal wrote %d blocks", got)
	}
	if !eng.GetRootRef().GetRootRef().EqualsRef(head) {
		t.Fatal("rehearsal moved the head")
	}

	// The committed World holds the write's link and not the rehearsal's.
	rd, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Discard()
	if hasRehearsalLink(ctx, t, rd, 0, 200) || !hasRehearsalLink(ctx, t, rd, 1, 200) {
		t.Fatal("committed World holds the wrong links")
	}
}

// TestRehearsalWarmsLockedWrite measures the writer slot on a World of
// production size in a store whose cache starts cold: every block's first
// read waits as a disk read does. A rehearsal before each write must leave
// the locked apply and commit no cold block, and the median slot hold must
// fall against writing without one.
func TestRehearsalWarmsLockedWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds a World of production size")
	}

	// Seed the World once; each run opens it fresh on a cold cache.
	ctx := t.Context()
	tbed, rootRef, err := seedWorld(ctx, worldSeedObjects)
	if err != nil {
		t.Fatal(err)
	}
	defer tbed.Release()
	const writes = 40
	const coldRead = 200 * time.Microsecond
	run := func(rehearse bool) (time.Duration, int64) {
		eng, cleanup := openSeededEngine(ctx, t, tbed, rootRef)
		defer cleanup()
		cold := newColdCache(coldRead)
		world_block.WrapBlockStore(eng, cold.readThrough())

		// Apply each write, rehearsed first when asked, timing the slot.
		holds := make([]time.Duration, 0, writes)
		for i := range writes {
			if rehearse {
				rtx, err := eng.NewRehearsalTransaction(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if err := applyRehearsalOp(ctx, rtx, i, worldSeedObjects); err != nil {
					t.Fatal(err)
				}
				if err := rtx.Commit(ctx); !errors.Is(err, world.ErrRehearsalCommit) {
					t.Fatal(err)
				}
			}
			holds = append(holds, timeLockedWrite(ctx, t, eng, cold, i, worldSeedObjects))
		}
		slices.Sort(holds)
		return holds[len(holds)/2], cold.slotReads.Load()
	}

	// Compare the two runs.
	baseHold, baseReads := run(false)
	hold, reads := run(true)
	t.Logf("without rehearsal: median slot hold %v, %d cold reads under the slot", baseHold, baseReads)
	t.Logf("with rehearsal: median slot hold %v, %d cold reads under the slot", hold, reads)
	if reads != 0 {
		t.Fatalf("locked writes read %d cold blocks after rehearsal", reads)
	}
	if hold >= baseHold {
		t.Fatalf("median slot hold %v did not fall from %v", hold, baseHold)
	}
}

// TestRehearsalScales holds a rehearsal's allocations and block reads flat
// between n and 10n objects the operation does not touch, beyond what the
// locked write of the same operation pays for the World tree's extra level.
func TestRehearsalScales(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds a World of production size")
	}

	// Measure one rehearsal and one locked write at each size.
	ctx := t.Context()
	type cost struct {
		allocs float64
		reads  uint64
	}
	measure := func(n int) (rehearsal, write cost) {
		tbed, rootRef, err := seedWorld(ctx, n)
		if err != nil {
			t.Fatal(err)
		}
		defer tbed.Release()
		eng, cleanup := openSeededEngine(ctx, t, tbed, rootRef)
		defer cleanup()
		rehearsal.allocs = testing.AllocsPerRun(20, func() {
			rctx, readCounter, _ := withWorldWriteBenchCounters(ctx)
			rtx, err := eng.NewRehearsalTransaction(rctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := applyRehearsalOp(rctx, rtx, 3, n); err != nil {
				t.Fatal(err)
			}
			if err := rtx.Commit(rctx); !errors.Is(err, world.ErrRehearsalCommit) {
				t.Fatal(err)
			}
			rehearsal.reads = readCounter.Snapshot().BlockReadCount
		})
		i := 0
		write.allocs = testing.AllocsPerRun(20, func() {
			wctx, readCounter, _ := withWorldWriteBenchCounters(ctx)
			if err := seedWorldTx(wctx, eng, func(ws world.WorldState) error {
				return applyRehearsalOp(wctx, ws, 3+i, n)
			}); err != nil {
				t.Fatal(err)
			}
			write.reads = readCounter.Snapshot().BlockReadCount
			i++
		})
		return rehearsal, write
	}
	small, smallWrite := measure(worldSeedObjects / 10)
	large, largeWrite := measure(worldSeedObjects)
	t.Logf("n=%d: rehearsal %.0f allocs, %d reads; write %.0f allocs, %d reads", worldSeedObjects/10, small.allocs, small.reads, smallWrite.allocs, smallWrite.reads)
	t.Logf("n=%d: rehearsal %.0f allocs, %d reads; write %.0f allocs, %d reads", worldSeedObjects, large.allocs, large.reads, largeWrite.allocs, largeWrite.reads)

	// The rehearsal reads no more of the larger tree than the write it warms,
	// and its allocations grow by the extra tree level alone: under twice,
	// where a cost in the World's size or its square root grows 10 or 3.2
	// times. The write's own allocations are no yardstick, since its commit
	// work does not depend on the World's size and can hide the level.
	allocGrowth := large.allocs / small.allocs
	readGrowth, writeReadGrowth := float64(large.reads)/float64(small.reads), float64(largeWrite.reads)/float64(smallWrite.reads)
	if allocGrowth >= 2 || readGrowth > writeReadGrowth*1.1 {
		t.Fatalf("rehearsal grew %.2fx allocs, %.2fx reads against the write's %.2fx reads", allocGrowth, readGrowth, writeReadGrowth)
	}
}

// coldCache models a store whose cache starts cold: the first read of each
// block waits as a disk read, later reads and written blocks are cached.
type coldCache struct {
	// wait is one cold read's duration.
	wait time.Duration
	// warm holds the cached refs.
	warm sync.Map
	// inSlot is set while a locked write holds the writer slot.
	inSlot atomic.Bool
	// slotReads counts cold reads while inSlot is set.
	slotReads atomic.Int64
}

// newColdCache returns an empty cold cache.
func newColdCache(wait time.Duration) *coldCache {
	return &coldCache{wait: wait}
}

// readThrough routes a store's reads and writes through the cache.
func (c *coldCache) readThrough() world_block.ReadThrough {
	return world_block.ReadThrough{
		Get: func(ctx context.Context, store block.StoreOps, ref *block.BlockRef) ([]byte, bool, error) {
			if _, cached := c.warm.LoadOrStore(ref.MarshalString(), struct{}{}); !cached {
				if c.inSlot.Load() {
					c.slotReads.Add(1)
				}
				time.Sleep(c.wait)
			}
			return store.GetBlock(ctx, ref)
		},
		Put: func(ref *block.BlockRef) { c.warm.Store(ref.MarshalString(), struct{}{}) },
	}
}

// timeLockedWrite applies write i under the writer slot and returns how long
// the slot was held.
func timeLockedWrite(ctx context.Context, t *testing.T, eng *world_block.Engine, cold *coldCache, i, n int) time.Duration {
	// Take the slot, apply and commit, counting cold reads meanwhile.
	t.Helper()
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	start := time.Now()
	cold.inSlot.Store(true)
	defer cold.inSlot.Store(false)
	if err := applyRehearsalOp(ctx, tx, i, n); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return time.Since(start)
}

// applyRehearsalOp links object i of n to another seeded object and bumps the
// linked object's revision, as an operation touching two objects does.
func applyRehearsalOp(ctx context.Context, ws world.WorldState, i, n int) error {
	// Link the pair.
	src, dst := rehearsalPair(i, n)
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(src, rehearsalLinkPred, dst, "")); err != nil {
		return err
	}

	// Bump the source's revision.
	obj, found, err := ws.GetObject(ctx, src)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}
	_, err = obj.IncrementRev(ctx)
	return err
}

// hasRehearsalLink reports whether ws holds operation i's link.
func hasRehearsalLink(ctx context.Context, t *testing.T, ws world.WorldState, i, n int) bool {
	t.Helper()
	src, dst := rehearsalPair(i, n)
	quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(src, rehearsalLinkPred, dst, ""), 1)
	if err != nil {
		t.Fatal(err)
	}
	return len(quads) != 0
}

// rehearsalPair spreads operation i across two of n seeded objects.
func rehearsalPair(i, n int) (string, string) {
	return worldSeedObjectKey(i * 7919 % n), worldSeedObjectKey((i*7919 + n/2) % n)
}

// BenchmarkSeededWrite measures the locked write of the measured operation on
// a World of production size.
func BenchmarkSeededWrite(b *testing.B) {
	benchmarkSeededWrite(b, false)
}

// BenchmarkSeededRehearsedWrite measures a rehearsal and then the locked write
// of the same operation on a World of production size.
func BenchmarkSeededRehearsedWrite(b *testing.B) {
	benchmarkSeededWrite(b, true)
}

// benchmarkSeededWrite applies one operation per iteration, rehearsed first
// when asked.
func benchmarkSeededWrite(b *testing.B, rehearse bool) {
	// Open the seeded World.
	ctx := b.Context()
	tbed, rootRef, err := seedWorld(ctx, worldSeedObjects)
	if err != nil {
		b.Fatal(err)
	}
	defer tbed.Release()
	eng, cleanup := openSeededEngine(ctx, b, tbed, rootRef)
	defer cleanup()

	// Apply one operation per iteration.
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		if rehearse {
			rtx, err := eng.NewRehearsalTransaction(ctx)
			if err != nil {
				b.Fatal(err)
			}
			if err := applyRehearsalOp(ctx, rtx, i, worldSeedObjects); err != nil {
				b.Fatal(err)
			}
			if err := rtx.Commit(ctx); !errors.Is(err, world.ErrRehearsalCommit) {
				b.Fatal(err)
			}
		}
		if err := seedWorldTx(ctx, eng, func(ws world.WorldState) error {
			return applyRehearsalOp(ctx, ws, i, worldSeedObjects)
		}); err != nil {
			b.Fatal(err)
		}
	}
}
