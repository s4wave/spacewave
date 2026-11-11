package kvtx

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"maps"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// testClock is a clock a test moves by hand.
type testClock struct {
	// mtx guards now.
	mtx sync.Mutex
	// now is the time the clock reads.
	now time.Time
}

// Now returns the clock's time.
func (c *testClock) Now() time.Time {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.now
}

// Add moves the clock forward by d.
func (c *testClock) Add(d time.Duration) {
	c.mtx.Lock()
	c.now = c.now.Add(d)
	c.mtx.Unlock()
}

// newLivenessVolume opens an empty Volume, adopted from birth, with the
// collector's frees set and a clock the test moves.
func newLivenessVolume(t *testing.T, frees bool) (*Volume, *testClock) {
	t.Helper()
	return openLivenessVolume(t, store_kvtx_inmem.NewStore(), frees)
}

// openLivenessVolume opens a Volume over store, as newLivenessVolume does.
func openLivenessVolume(t *testing.T, store *store_kvtx_inmem.Store, frees bool) (*Volume, *testClock) {
	t.Helper()
	clk := &testClock{now: time.Unix(1_800_000_000, 0)}
	v, err := NewVolume(t.Context(), "test/liveness", store_kvkey.NewDefaultKVKey(), store, &store_kvtx.Config{CollectorFrees: frees}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	v.clock = clk.Now
	t.Cleanup(func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	})
	return v, clk
}

// openUnadoptedVolume opens a Volume over store, which predates adoption,
// with the collector's frees on and no background adoption, so the test
// drives the adoption itself.
func openUnadoptedVolume(t *testing.T, store *store_kvtx_inmem.Store) (*Volume, *testClock) {
	t.Helper()
	v, clk := openLivenessVolume(t, store, false)
	v.collector = v.newCollector(true, 0)
	return v, clk
}

// testTree is a set of blocks a test builds bottom up, each named.
type testTree map[string]*block.PutBatchEntry

// add builds the block name naming the blocks children name, which add
// built before, and returns its ref.
func (tr testTree) add(t *testing.T, name string, children ...string) *block.BlockRef {
	t.Helper()
	refs := make([]*block.BlockRef, 0, len(children))
	data := []byte("block " + name)
	for _, child := range children {
		refs = append(refs, tr[child].Ref)
		data = append(data, tr[child].Ref.MarshalString()...)
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	tr[name] = &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}
	return ref
}

// entries returns the blocks names name.
func (tr testTree) entries(names ...string) []*block.PutBatchEntry {
	out := make([]*block.PutBatchEntry, len(names))
	for i, name := range names {
		out[i] = tr[name]
	}
	return out
}

// collectAll runs collection passes until one reports no more work.
func collectAll(t *testing.T, v *Volume) block_gc.CollectResult {
	t.Helper()
	var total block_gc.CollectResult
	for range 100 {
		res, err := v.Collect(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		total.Netted += res.Netted
		total.Freed += res.Freed
		total.Kept += res.Kept
		total.Parked += res.Parked
		total.Requeued += res.Requeued
		total.Leaked += res.Leaked
		total.Stranded += res.Stranded
		total.Lapsed += res.Lapsed
		total.Underflows += res.Underflows
		if !res.More {
			return total
		}
	}
	t.Fatal("collection never finished")
	return total
}

// requireStored checks that each named block of tr is stored or gone.
func requireStored(t *testing.T, v *Volume, tr testTree, want bool, names ...string) {
	t.Helper()
	for _, name := range names {
		_, found, err := v.GetBlock(t.Context(), tr[name].Ref)
		if err != nil {
			t.Fatal(err)
		}
		if found != want {
			t.Fatalf("block %s stored=%v, want %v", name, found, want)
		}
	}
}

// requireAudit checks that an audit finds no live block queued and leaked
// blocks.
func requireAudit(t *testing.T, v *Volume, leaked uint64) {
	t.Helper()
	res, err := v.Audit(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.LiveQueued != 0 || res.Leaked != leaked || res.Undercount != 0 || res.Unlisted != 0 || res.CascadeMiscount != 0 || res.UnlistedCascades != 0 {
		t.Fatalf("audit live queued %d (sample %v) leaked %d undercount %d (sample %v) unlisted %d cascade miscount %d unlisted cascades %d, want 0, %d, 0, 0, 0 and 0",
			res.LiveQueued, res.LiveQueuedSample, res.Leaked, res.Undercount, res.UndercountSample, res.Unlisted, res.CascadeMiscount, res.UnlistedCascades, leaked)
	}
}

// TestLivenessRunThrough stores a tree, swaps the root to a second tree that
// shares a subtree, collects, and checks that the grace period keeps the old
// tree, that afterwards only what the live root reaches is stored, and that a
// hole under a dead page frees nothing below it while the audit reports it.
func TestLivenessRunThrough(t *testing.T) {
	// Store R1 -> C -> {E, F}, F -> G under main.
	v, clk := newLivenessVolume(t, true)
	ctx := t.Context()
	tr := testTree{}
	tr.add(t, "G")
	tr.add(t, "F", "G")
	tr.add(t, "E")
	tr.add(t, "C", "E", "F")
	r1 := tr.add(t, "R1", "C")
	if err := v.SetBucketRoots(ctx, "b", tr.entries("G", "F", "E", "C", "R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
		t.Fatal(err)
	}

	// Move main to R2 -> C2 -> {F}, sharing F.
	tr.add(t, "C2", "F")
	r2 := tr.add(t, "R2", "C2")
	if err := v.SetBucketRoots(ctx, "b", tr.entries("C2", "R2"), []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
		t.Fatal(err)
	}

	// Inside the grace period nothing is freed.
	if res := collectAll(t, v); res.Freed != 0 {
		t.Fatalf("freed %d inside the grace period", res.Freed)
	}
	requireStored(t, v, tr, true, "R1", "C", "E", "F", "G", "R2", "C2")

	// After it, the old tree's own blocks go and the shared subtree stays.
	clk.Add(block_gc.DefaultGrace + time.Second)
	if res := collectAll(t, v); res.Freed != 3 || res.Underflows != 0 {
		t.Fatalf("freed %d with %d underflows, want 3 and none", res.Freed, res.Underflows)
	}
	requireStored(t, v, tr, false, "R1", "C", "E")
	requireStored(t, v, tr, true, "R2", "C2", "F", "G")
	requireAudit(t, v, 0)

	// Store R3 -> D -> {H} under side, then lose D with its child list.
	tr.add(t, "H")
	tr.add(t, "D", "H")
	r3 := tr.add(t, "R3", "D")
	if err := v.SetBucketRoots(ctx, "b", tr.entries("H", "D", "R3"), []block.NamedRoot{{Name: "side", Ref: r3}}); err != nil {
		t.Fatal(err)
	}
	punchHole(t, v, tr["D"].Ref)

	// Releasing side frees R3, and the cascade stops at the hole: H stays,
	// and the audit reports it leaked.
	if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "side", Previous: r3}}); err != nil {
		t.Fatal(err)
	}
	clk.Add(block_gc.DefaultGrace + time.Second)
	if res := collectAll(t, v); res.Freed != 1 || res.Leaked != 1 {
		t.Fatalf("freed %d leaked %d, want 1 and 1", res.Freed, res.Leaked)
	}
	requireStored(t, v, tr, false, "R3")
	requireStored(t, v, tr, true, "H", "R2", "C2", "F", "G")
	requireAudit(t, v, 1)

	// A released chain frees whole in one pass, though its cascade queues
	// children on both sides of the walk's position.
	names := []string{"L0"}
	tr.add(t, "L0")
	for i := 1; i < 32; i++ {
		names = append(names, "L"+strconv.Itoa(i))
		tr.add(t, names[i], names[i-1])
	}
	top := tr[names[len(names)-1]].Ref
	if err := v.SetBucketRoots(ctx, "b", tr.entries(names...), []block.NamedRoot{{Name: "chain", Ref: top}}); err != nil {
		t.Fatal(err)
	}
	if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "chain", Previous: top}}); err != nil {
		t.Fatal(err)
	}
	clk.Add(block_gc.DefaultGrace + time.Second)
	res, err := v.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Freed != uint64(len(names)) || res.More {
		t.Fatalf("one pass freed %d of the %d-block chain, more %v", res.Freed, len(names), res.More)
	}
	requireStored(t, v, tr, false, names...)

	// A tree whose bytes the caller keeps lands as external entries: the
	// Volume holds its child lists and roots but not its bytes, as it holds
	// evicted blocks, and the collector keeps what the root reaches.
	tr.add(t, "X1")
	x0 := tr.add(t, "X0", "X1")
	external := tr.entries("X1", "X0")
	for i, entry := range external {
		copied := *entry
		copied.External = true
		external[i] = &copied
	}
	if err := v.SetBucketRoots(ctx, "b", external, []block.NamedRoot{{Name: "ext", Ref: x0}}); err != nil {
		t.Fatal(err)
	}
	requireStored(t, v, tr, false, "X0", "X1")
	clk.Add(block_gc.DefaultGrace + time.Second)
	if res := collectAll(t, v); res.Freed != 0 {
		t.Fatalf("freed %d of a rooted external tree", res.Freed)
	}
	requireAudit(t, v, 1)

	// A later put of the bytes stores them, and releasing the root frees the
	// tree after the grace period.
	if err := putBatch(ctx, v, tr.entries("X1", "X0")); err != nil {
		t.Fatal(err)
	}
	requireStored(t, v, tr, true, "X0", "X1")
	if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "ext", Previous: x0}}); err != nil {
		t.Fatal(err)
	}
	clk.Add(block_gc.DefaultGrace + time.Second)
	if res := collectAll(t, v); res.Freed != 2 {
		t.Fatalf("freed %d of the released external tree, want 2", res.Freed)
	}
	requireStored(t, v, tr, false, "X0", "X1")

	// Each entry point costs the same over 1x and 10x stored blocks: its
	// transactions, reads, steps and writes do not grow with the Volume, nor
	// does the longest write transaction it holds, as the Phase 1 path walk
	// says. The index pass reads each young journal record once, so only its
	// longest transaction is bounded.
	small, large := measureLivenessPaths(t, 300), measureLivenessPaths(t, 3000)
	within := func(l, s uint64) bool { return l <= s+s/4+4 }
	for name, s := range small.counts {
		l := large.counts[name]
		t.Logf("%s: %+v over 1x, %+v over 10x", name, s, l)
		total := name == "index" || (within(l.Total(), s.Total()) && l.Txns <= s.Txns+1)
		if !total || !within(l.MaxWriteOps, s.MaxWriteOps) {
			t.Errorf("%s over 10x stored blocks cost %+v, over 1x %+v", name, l, s)
		}
	}

	// A commit of three blocks reads each block's child list and bytes once,
	// beyond the state and the root it moves.
	if c := small.counts["commit"]; c.Reads+c.Steps > 2*3+4 {
		t.Errorf("a commit of three blocks read %d records and stepped %d", c.Reads, c.Steps)
	}

	// A staged put copies nothing of what the stage holds, and a commit waits
	// on no scan of the process's pins and stages.
	t.Logf("wide stage: %d bytes over 1x, %d over 10x", small.allocs["wide stage"], large.allocs["wide stage"])
	if s, l := small.allocs["wide stage"], large.allocs["wide stage"]; l > 2*s+16<<10 {
		t.Errorf("a put through a stage holding 10x tops allocated %d bytes, over 1x %d", l, s)
	}
	// Freeing a block holds the writer no longer for twice its children.
	s, sd := measureWideFree(t, 10_000, false)
	l, ld := measureWideFree(t, 20_000, false)
	if !within(l, s) {
		t.Errorf("freeing a block of 20,000 children made a transaction of %d operations, of 10,000 %d", l, s)
	} else {
		t.Logf("wide free: longest transaction %d operations in %v over 10,000 children, %d in %v over 20,000", s, sd, l, ld)
	}

	// A collector that finds a writer waiting stops at the next block or
	// release page, whatever it frees: a page of 1,024 children is about
	// 5,100 operations.
	s, sd = measureWideFree(t, 5_000, true)
	l, ld = measureWideFree(t, 10_000, true)
	if !within(l, s) || l > 6_000 {
		t.Errorf("beside a waiting writer, freeing a block of 10,000 children made a transaction of %d operations, of 5,000 %d", l, s)
	} else {
		t.Logf("wide free beside a waiting writer: longest transaction %d operations in %v over 5,000 children, %d in %v over 10,000", s, sd, l, ld)
	}

	// A page replaced by one sharing its children costs no read and no write
	// per shared child, in the net or the free: the new page's up pair and
	// the old page's cascade down cancel in the slice.
	net31, free31 := measureReplacedPage(t, 31)
	for _, shared := range []int{63, 127} {
		net, free := measureReplacedPage(t, shared)
		t.Logf("replaced page of %d shared children: net %d reads %d writes, free %d reads %d writes; of 31: net %d/%d, free %d/%d",
			shared, net.Reads+net.Steps, net.Writes, free.Reads+free.Steps, free.Writes,
			net31.Reads+net31.Steps, net31.Writes, free31.Reads+free31.Steps, free31.Writes)
		if net.Reads+net.Steps != net31.Reads+net31.Steps || net.Writes != net31.Writes ||
			free.Reads+free.Steps != free31.Reads+free31.Steps || free.Writes != free31.Writes {
			t.Errorf("a replaced page of %d shared children cost more than one of 31", shared)
		}
	}

	// A staged batch's lookups keep no commit waiting, at 3 or 32 children an
	// entry.
	if s, l := measureStageLookupWait(t, 3), measureStageLookupWait(t, 32); l > 2*s+time.Millisecond {
		t.Errorf("a commit beside a staged batch of 32 children an entry waited %v, of 3 %v", l, s)
	} else {
		t.Logf("stage lookups: longest commit %v beside 3 children an entry, %v beside 32", s, l)
	}
	t.Logf("deferred: %v over 1x, %v over 10x", small.times["deferred"], large.times["deferred"])
	if s, l := small.times["deferred"], large.times["deferred"]; l > 2*s+200*time.Microsecond {
		t.Errorf("a commit beside 10x pins and stages took %v, beside 1x %v", l, s)
	}
}

// livenessCosts are what the entry points cost on one Volume.
type livenessCosts struct {
	// counts are the store operations of each entry point.
	counts map[string]kvtx_kvtest.Counts
	// allocs are the fewest bytes one call of an entry point allocated.
	allocs map[string]uint64
	// times are the median durations of an entry point's calls.
	times map[string]time.Duration
}

// measureLivenessPaths opens a Volume that frees at once over a counting
// store holding n stored blocks under roots, and returns what each entry point
// the Phase 1 path walk names cost on it: a commit of a small tree with its
// root move while proofs of n/100 roots were marked, a mark of three proofs, a
// read of a block and its child list, a reader pin, a staged put, a cache
// fill, a collector pass over 2n young journal records and the pass after it,
// which collects the commit's tree, a put through a stage that holds n tops,
// and a commit beside 10n pins and 10n open stages.
func measureLivenessPaths(t *testing.T, n int) livenessCosts {
	// Open the Volume over a counting store.
	t.Helper()
	ctx := t.Context()
	cs := kvtx_kvtest.NewCountingStore(store_kvtx_inmem.NewStore())
	v, err := NewVolume(ctx, "test/scaling", store_kvkey.NewDefaultKVKey(), cs, &store_kvtx.Config{CollectorFrees: true}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	clk := &testClock{now: time.Unix(1_800_000_000, 0)}
	v.clock = clk.Now
	t.Cleanup(func() { _ = v.Close() })

	// Store n leaves, a hundred under each parent and each parent under a
	// root, and net their journal.
	tr := testTree{}
	var parents []*block.BlockRef
	for i := 0; i < n; i += 100 {
		var names []string
		for j := i; j < min(i+100, n); j++ {
			names = append(names, "leaf "+strconv.Itoa(j))
			tr.add(t, names[len(names)-1])
		}
		parent := "parent " + strconv.Itoa(i)
		p := tr.add(t, parent, names...)
		parents = append(parents, p)
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(names, parent)...), []block.NamedRoot{{Name: parent, Ref: p}}); err != nil {
			t.Fatal(err)
		}
	}
	clk.Add(block_gc.DefaultGrace + time.Second)
	collectAll(t, v)

	// Measure each entry point.
	costs := livenessCosts{
		counts: make(map[string]kvtx_kvtest.Counts),
		allocs: make(map[string]uint64),
		times:  make(map[string]time.Duration),
	}
	measure := func(name string, fn func() error) {
		t.Helper()
		cs.ResetMax()
		before := cs.Counts()
		if err := fn(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		costs.counts[name] = cs.Counts().Sub(before)
	}
	if err := v.MarkRootsComplete(ctx, parents); err != nil {
		t.Fatal(err)
	}
	tr.add(t, "C")
	tr.add(t, "P", "C")
	r := tr.add(t, "R", "P")
	measure("commit", func() error {
		return v.SetBucketRoots(ctx, "b", tr.entries("C", "P", "R"), []block.NamedRoot{{Name: "main", Ref: r}})
	})
	measure("mark", func() error {
		return v.MarkRootsComplete(ctx, []*block.BlockRef{tr["C"].Ref, tr["P"].Ref, r})
	})
	measure("read", func() error {
		if _, _, err := v.GetBlock(ctx, tr["leaf 0"].Ref); err != nil {
			return err
		}
		_, _, err := v.GetChildren(ctx, tr["parent 0"].Ref)
		return err
	})
	measure("pin", func() error {
		unpin, err := v.PinBucketRoot(ctx, r)
		if err != nil {
			return err
		}
		unpin()
		return v.withDirectAtomic(ctx, nil)
	})
	tr.add(t, "S1")
	tr.add(t, "S2", "S1")
	measure("stage", func() error {
		stage, release, err := v.OpenStage(ctx)
		if err != nil {
			return err
		}
		_, err = v.PrepareStagedBlockBatch(ctx, stage, tr.entries("S1", "S2"))
		release()
		if err != nil {
			return err
		}
		return v.withDirectAtomic(ctx, nil)
	})
	tr.add(t, "F")
	measure("fill", func() error {
		return v.withDirectFill(ctx, func(p *physicalTx) error {
			_, err := block_gc.LandFill(ctx, p.rec, p.blocks, v.now(), tr.entries("F"))
			return err
		})
	})

	// Release the commit's tree, then move a root back and forth 2n times
	// while the release ages: a pass reads each young record once, and the
	// pass that collects the tree reads only the records since.
	if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
		t.Fatal(err)
	}
	clk.Add(block_gc.DefaultGrace / 2)
	churn := []*block.BlockRef{tr.add(t, "X"), tr.add(t, "Y")}
	if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "Y"), []block.NamedRoot{{Name: "churn", Ref: churn[0]}}); err != nil {
		t.Fatal(err)
	}
	moves := 0
	move := func(k int) {
		t.Helper()
		for range k {
			from, to := churn[moves%2], churn[(moves+1)%2]
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "churn", Ref: to, Previous: from}}); err != nil {
				t.Fatal(err)
			}
			moves++
		}
	}
	move(2 * n)
	measure("index", func() error {
		collectAll(t, v)
		return nil
	})
	clk.Add(block_gc.DefaultGrace/2 + time.Second)
	move(4)
	measure("collect", func() error {
		collectAll(t, v)
		return nil
	})

	// The tree ripens at the journal's end its release netted at, so it
	// goes once the churn ages too.
	clk.Add(block_gc.DefaultGrace + time.Second)
	collectAll(t, v)
	requireStored(t, v, tr, false, "C", "P", "R", "S1", "S2", "F")

	// Put pairs of blocks through a stage that holds n tops of its own.
	wide, releaseWide, err := v.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseWide()
	for i := 0; i < n; i += 300 {
		var names []string
		for j := i; j < min(i+300, n); j++ {
			names = append(names, "top "+strconv.Itoa(j))
			tr.add(t, names[len(names)-1])
		}
		if _, err := v.PrepareStagedBlockBatch(ctx, wide, tr.entries(names...)); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 5 {
		lo, hi := "W"+strconv.Itoa(i), "W top "+strconv.Itoa(i)
		tr.add(t, lo)
		tr.add(t, hi, lo)
		put := func() error {
			_, err := v.PrepareStagedBlockBatch(ctx, wide, tr.entries(lo, hi))
			return err
		}
		var bytes uint64
		if i == 0 {
			bytes = allocated(func() { measure("wide stage", put) })
		} else {
			bytes = allocated(func() {
				if err := put(); err != nil {
					t.Fatal(err)
				}
			})
		}
		if prev, ok := costs.allocs["wide stage"]; !ok || bytes < prev {
			costs.allocs["wide stage"] = bytes
		}
	}

	// Commit beside 10n reader pins and 10n open stages, none released.
	for i := range 10 * n {
		ref, err := block.BuildBlockRef([]byte("pin "+strconv.Itoa(i)), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := v.pinRoot(ctx, ref, true); err != nil {
			t.Fatal(err)
		}
		if _, _, err := v.OpenStage(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var prev *block.BlockRef
	times := make([]time.Duration, 0, 15)
	for i := range 15 {
		name := "D" + strconv.Itoa(i)
		ref := tr.add(t, name)
		commit := func() error {
			return v.SetBucketRoots(ctx, "b", tr.entries(name), []block.NamedRoot{{Name: "deferred", Ref: ref, Previous: prev}})
		}
		start := time.Now()
		if i == 0 {
			measure("deferred", commit)
		} else if err := commit(); err != nil {
			t.Fatal(err)
		}
		times = append(times, time.Since(start))
		prev = ref
	}
	slices.Sort(times)
	costs.times["deferred"] = times[len(times)/2]
	return costs
}

// allocated returns the bytes the process allocated while fn ran.
func allocated(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// errMidRelease is the failure stopMidRelease returns.
var errMidRelease = errors.New("stopped mid-release")

// stopMidRelease runs a collector's transactions until one leaves a paged
// release in progress, a paged re-hold when rehold is set, or a cascade
// record due when cascade is set, then fails every later one, as a process
// that dies there.
type stopMidRelease struct {
	block_gc.Store
	// v is the Volume, whose state it reads after each transaction.
	v *Volume
	// rehold selects a paged re-hold instead of a release.
	rehold bool
	// cascade selects a due cascade record instead of a paged cascade.
	cascade bool
	// stopped records that a transaction left one in progress.
	stopped bool
}

// Update runs the transaction unless a release is in progress.
func (s *stopMidRelease) Update(ctx context.Context, fn func(rec *block_gc.Records, blocks block.StoreOps) error) error {
	if s.stopped {
		return errMidRelease
	}
	if err := s.Store.Update(ctx, fn); err != nil {
		return err
	}
	return s.v.readRecords(ctx, func(rec *block_gc.Records) error {
		state, err := rec.GetState(ctx)
		r := state.GetRelease()
		s.stopped = r != nil && r.GetRehold() == s.rehold
		if s.cascade {
			s.stopped = state.GetCascades() != 0
		}
		return err
	})
}

// livenessCascades returns the cascade records the state of v counts.
func livenessCascades(t *testing.T, v *Volume) uint64 {
	t.Helper()
	var due uint64
	err := v.readRecords(t.Context(), func(rec *block_gc.Records) error {
		state, err := rec.GetState(t.Context())
		due = state.GetCascades()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return due
}

// errPaceLimit is the failure paceProbe returns past its limit.
var errPaceLimit = errors.New("collection passed its transaction limit")

// paceProbe runs a collector's transactions, calling after following each,
// as a store whose commit costs time or a writer that commits between the
// collector's slices, and fails every transaction past limit.
type paceProbe struct {
	block_gc.Store
	// limit is how many transactions the probe runs.
	limit int
	// txs counts the transactions run.
	txs int
	// after runs after each transaction.
	after func()
}

// Update runs the transaction and after, or fails past the limit.
func (p *paceProbe) Update(ctx context.Context, fn func(rec *block_gc.Records, blocks block.StoreOps) error) error {
	if p.txs++; p.txs > p.limit {
		return errPaceLimit
	}
	err := p.Store.Update(ctx, fn)
	p.after()
	return err
}

// collectThrough runs passes of a collector of probe until one reports no
// more work, failing the test if the collection passes the probe's limit,
// and returns how many blocks it freed.
func collectThrough(t *testing.T, probe *paceProbe, opts block_gc.CollectorOptions) uint64 {
	t.Helper()
	c := block_gc.NewCollector(probe, opts)
	var freed uint64
	for {
		res, err := c.Collect(t.Context())
		if err != nil {
			t.Fatalf("after %d transactions, %d blocks freed: %v", probe.txs-1, freed+res.Freed, err)
		}
		if freed += res.Freed; !res.More {
			t.Logf("collected in %d transactions", probe.txs)
			return freed
		}
	}
}

// measureWideFree stores a block naming children leaves under a root,
// releases it, and returns the most operations one transaction of the
// collection that frees it made, and the longest that transaction held the
// store's writer, by a collector that finds a writer waiting when waiting is
// set.
func measureWideFree(t *testing.T, children int, waiting bool) (uint64, time.Duration) {
	// Store the block and its leaves, then release it.
	t.Helper()
	ctx := t.Context()
	cs := kvtx_kvtest.NewCountingStore(store_kvtx_inmem.NewStore())
	v, err := NewVolume(ctx, "test/wide-free", store_kvkey.NewDefaultKVKey(), cs, &store_kvtx.Config{CollectorFrees: true}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	clk := &testClock{now: time.Unix(1_800_000_000, 0)}
	v.clock = clk.Now
	t.Cleanup(func() { _ = v.Close() })
	tr := testTree{}
	leaves := make([]string, children)
	for i := range leaves {
		leaves[i] = "leaf " + strconv.Itoa(i)
		tr.add(t, leaves[i])
	}
	w := tr.add(t, "W", leaves...)
	if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "W")...), []block.NamedRoot{{Name: "w", Ref: w}}); err != nil {
		t.Fatal(err)
	}
	if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "w", Previous: w}}); err != nil {
		t.Fatal(err)
	}

	// Collect it all once the release ages.
	clk.Add(block_gc.DefaultGrace + time.Second)
	cs.ResetMax()
	store := &timedStore{Store: collectorStore{v: v}}
	opts := block_gc.CollectorOptions{Frees: true, Now: v.now}
	if waiting {
		opts.ForegroundWaiting = func() bool { return true }
	}
	c := block_gc.NewCollector(store, opts)
	var freed uint64
	for {
		res, err := c.Collect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if freed += res.Freed; !res.More {
			break
		}
	}
	if freed != uint64(children)+1 { //nolint:gosec // a test size.
		t.Fatalf("freed %d of a block and its %d children", freed, children)
	}
	return cs.Counts().MaxWriteOps, store.longest
}

// measureReplacedPage stores a page naming shared leaves and one leaf of its
// own under a root, nets it, then moves the root onto a new page naming the
// same shared leaves and another leaf of its own, and returns what netting
// that commit and then freeing the old page cost the store.
func measureReplacedPage(t *testing.T, shared int) (net, free kvtx_kvtest.Counts) {
	// Store the page under main and net it.
	t.Helper()
	ctx := t.Context()
	cs := kvtx_kvtest.NewCountingStore(store_kvtx_inmem.NewStore())
	v, err := NewVolume(ctx, "test/replaced-page", store_kvkey.NewDefaultKVKey(), cs, &store_kvtx.Config{CollectorFrees: true}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	clk := &testClock{now: time.Unix(1_800_000_000, 0)}
	v.clock = clk.Now
	t.Cleanup(func() { _ = v.Close() })
	tr := testTree{}
	leaves := make([]string, shared)
	for i := range leaves {
		leaves[i] = "leaf " + strconv.Itoa(i)
		tr.add(t, leaves[i])
	}
	tr.add(t, "U")
	tr.add(t, "U2")
	old := tr.add(t, "P", append(slices.Clone(leaves), "U")...)
	if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "U", "P")...), []block.NamedRoot{{Name: "main", Ref: old}}); err != nil {
		t.Fatal(err)
	}
	clk.Add(block_gc.DefaultGrace + time.Second)
	collectAll(t, v)

	// Replace it, and measure the net with frees off, then the free.
	next := tr.add(t, "P2", append(slices.Clone(leaves), "U2")...)
	if err := v.SetBucketRoots(ctx, "b", tr.entries("U2", "P2"), []block.NamedRoot{{Name: "main", Ref: next, Previous: old}}); err != nil {
		t.Fatal(err)
	}
	clk.Add(block_gc.DefaultGrace + time.Second)
	run := func(frees bool) kvtx_kvtest.Counts {
		t.Helper()
		before := cs.Counts()
		c := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{Frees: frees, Now: v.now})
		for {
			res, err := c.Collect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !res.More {
				return cs.Counts().Sub(before)
			}
		}
	}
	net, free = run(false), run(true)
	requireStored(t, v, tr, false, "P", "U")
	requireStored(t, v, tr, true, append(slices.Clone(leaves), "U2", "P2")...)
	return net, free
}

// timedStore runs a collector's transactions and records the longest one,
// from its start to its commit: the longest a writer behind it waits.
type timedStore struct {
	block_gc.Store
	// longest is the longest transaction.
	longest time.Duration
}

// Update runs the transaction and records its time.
func (s *timedStore) Update(ctx context.Context, fn func(rec *block_gc.Records, blocks block.StoreOps) error) error {
	start := time.Now()
	err := s.Store.Update(ctx, fn)
	s.longest = max(s.longest, time.Since(start))
	return err
}

// measureStageLookupWait looks up the roots a staged batch of 4,096 entries
// of children children each touches, again and again, and returns the
// longest a changing commit took meanwhile.
func measureStageLookupWait(t *testing.T, children int) time.Duration {
	// Name a batch the stage would put.
	t.Helper()
	v, _ := newLivenessVolume(t, false)
	ctx := t.Context()
	stage, release, err := v.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	entries := make([]*block.PutBatchEntry, 4096)
	for i := range entries {
		refs := make([]*block.BlockRef, children)
		for j := range refs {
			if refs[j], err = block.BuildBlockRef([]byte("child "+strconv.Itoa(i*children+j)), nil); err != nil {
				t.Fatal(err)
			}
		}
		ref, err := block.BuildBlockRef([]byte("entry "+strconv.Itoa(i)), nil)
		if err != nil {
			t.Fatal(err)
		}
		entries[i] = &block.PutBatchEntry{Ref: ref, Data: []byte("x"), Refs: refs, RefsKnown: true}
	}

	// Look the batch up until the commits end.
	done := make(chan struct{})
	looked := make(chan struct{})
	go func() {
		defer close(looked)
		for {
			select {
			case <-done:
				return
			default:
			}
			v.stageHeld(stage, newStageNames(stage, entries))
		}
	}()

	// Commit a root move twenty times meanwhile, timing each.
	tr := testTree{}
	var prev *block.BlockRef
	var longest time.Duration
	for i := range 20 {
		name := "c" + strconv.Itoa(i)
		ref := tr.add(t, name)
		start := time.Now()
		if err := v.SetBucketRoots(ctx, "b", tr.entries(name), []block.NamedRoot{{Name: "c", Ref: ref, Previous: prev}}); err != nil {
			t.Fatal(err)
		}
		longest = max(longest, time.Since(start))
		prev = ref
	}
	close(done)
	<-looked
	return longest
}

// punchHole loses ref from the store with its child list and count, as
// damage to the store or a partial copy does.
func punchHole(t *testing.T, v *Volume, ref *block.BlockRef) {
	t.Helper()
	err := v.withDirectAtomic(t.Context(), func(p *physicalTx) error {
		if err := p.rec.DeleteChildren(t.Context(), ref); err != nil {
			return err
		}
		if err := p.rec.SetHolders(t.Context(), ref, 1, 0); err != nil {
			return err
		}
		return p.blocks.RmBlock(t.Context(), ref)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestLivenessAdversarial tries to make the collector free a live block: a
// reader pinning an old root during a swap and a reader whose lease lapses,
// two writers moving one root, a root moved away and back within the grace
// period, a release and a birth that share a child in different intervals,
// and a Volume that predates adoption. It frees no live block in any.
func TestLivenessAdversarial(t *testing.T) {
	t.Run("reader pin", func(t *testing.T) {
		// Store R1 -> E under main and pin it as a reader that saw it.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E")
		r1 := tr.add(t, "R1", "E")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("E", "R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		unpin, err := v.PinBucketRoot(ctx, r1)
		if err != nil {
			t.Fatal(err)
		}

		// Swap main away; the pin, renewed by its live holder, keeps the old
		// tree past the grace period.
		r2 := tr.add(t, "R2")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R1", "E", "R2")

		// The pin released, the old tree goes after the grace period.
		unpin()
		if err := v.withDirectAtomic(ctx, nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R1", "E")
		requireStored(t, v, tr, true, "R2")
		requireAudit(t, v, 0)
	})

	t.Run("lapsed lease", func(t *testing.T) {
		// A pin whose process died lapses; the collector releases it, and
		// frees the old tree only a grace period later.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		r1 := tr.add(t, "R1")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		if _, err := v.PinBucketRoot(ctx, r1); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		forgetPins(t, v)
		clk.Add(rootLease + time.Second)
		if res := collectAll(t, v); res.Lapsed != 1 || res.Freed != 0 {
			t.Fatalf("lapsed %d freed %d, want 1 and 0", res.Lapsed, res.Freed)
		}
		requireStored(t, v, tr, true, "R1")
		clk.Add(block_gc.DefaultGrace)
		if res := collectAll(t, v); res.Freed != 1 {
			t.Fatalf("freed %d after the lapsed pin's grace, want 1", res.Freed)
		}
	})

	t.Run("two writers", func(t *testing.T) {
		// Writer B moves main from R1 while writer A still expects R1.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		r1 := tr.add(t, "R1")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		r2 := tr.add(t, "R2")
		r3 := tr.add(t, "R3")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
			t.Fatal(err)
		}

		// A loses: its change is refused whole and stores nothing.
		err := v.SetBucketRoots(ctx, "b", tr.entries("R3"), []block.NamedRoot{{Name: "main", Ref: r3, Previous: r1}})
		if !errors.Is(err, block.ErrRootMoved) {
			t.Fatalf("stale move = %v, want ErrRootMoved", err)
		}
		requireStored(t, v, tr, false, "R3")

		// The winner's root survives collection; the old root goes.
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R2")
		requireStored(t, v, tr, false, "R1")
		requireAudit(t, v, 0)
	})

	for _, behind := range []int{0, 600} {
		t.Run("moved away and back behind "+strconv.Itoa(behind), func(t *testing.T) {
			// main moves R1 to R2 and back to R1 inside the grace period,
			// behind more records than one net slice holds; a pass that nets
			// only the first move leaves R1 and E Dead, and must not free
			// them, since they ripen at the journal's end, past the move
			// back.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			tr.add(t, "E")
			r1 := tr.add(t, "R1", "E")
			r2 := tr.add(t, "R2")
			if err := v.SetBucketRoots(ctx, "b", tr.entries("E", "R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
				t.Fatal(err)
			}
			if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace / 2)
			churn := []*block.BlockRef{tr.add(t, "X"), tr.add(t, "Y")}
			if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "Y"), []block.NamedRoot{{Name: "churn", Ref: churn[0]}}); err != nil {
				t.Fatal(err)
			}
			for i := range behind {
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "churn", Ref: churn[(i+1)%2], Previous: churn[i%2]}}); err != nil {
					t.Fatal(err)
				}
			}
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: r1, Previous: r2}}); err != nil {
				t.Fatal(err)
			}

			// Net only the first move: R1 is queued and kept by the later
			// record.
			clk.Add(block_gc.DefaultGrace/2 + time.Second)
			if res := collectAll(t, v); res.Freed != 0 || res.Kept == 0 {
				t.Fatalf("freed %d kept %d, want 0 freed and R1 kept", res.Freed, res.Kept)
			}
			requireStored(t, v, tr, true, "R1", "E", "R2")

			// Once the move back nets, R2 goes and R1's tree stays.
			clk.Add(block_gc.DefaultGrace)
			collectAll(t, v)
			requireStored(t, v, tr, true, "R1", "E")
			requireStored(t, v, tr, false, "R2")
			requireAudit(t, v, 0)

			// R1's tree goes once main leaves it.
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r1}}); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
			requireStored(t, v, tr, false, "R1", "E")
		})
	}

	t.Run("release and birth in different intervals", func(t *testing.T) {
		// R1 -> C -> F loses main; just inside its grace period a new R2
		// names F under another root. Netting the release alone brings R1,
		// C and F to zero and frees none, since they ripen at the journal's
		// end, past the birth. Once the birth nets, F keeps its new holder,
		// and R1 and C go.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "F")
		tr.add(t, "C", "F")
		r1 := tr.add(t, "R1", "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("F", "C", "R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		r2 := tr.add(t, "R2", "F")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "other", Ref: r2}}); err != nil {
			t.Fatal(err)
		}

		// Net the release only.
		clk.Add(2 * time.Second)
		if res := collectAll(t, v); res.Freed != 0 {
			t.Fatalf("freed %d before the birth netted", res.Freed)
		}
		requireStored(t, v, tr, true, "R1", "C", "F", "R2")

		// Once the birth nets, F keeps its new holder.
		clk.Add(block_gc.DefaultGrace)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R1", "C")
		requireStored(t, v, tr, true, "F", "R2")
		requireAudit(t, v, 0)
	})

	t.Run("re-put of a queued block", func(t *testing.T) {
		// R -> X loses main, and just inside its grace period a writer puts X
		// again, as a writer that puts one tree across several batches does
		// for a block it already stored. Netting the release brings R and X
		// to zero, ripe at the journal's end, past the re-put, so a parent
		// the writer puts later still finds X, and R goes once the re-put
		// ages.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		if _, err := v.PutBlockBatch(ctx, tr.entries("X")); err != nil {
			t.Fatal(err)
		}

		// Net the release only.
		clk.Add(2 * time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")

		// The writer's parent names X under main, and X stays.
		p := tr.add(t, "P", "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "X")
		requireStored(t, v, tr, false, "R")
		requireAudit(t, v, 0)
	})

	t.Run("put again after the release netted", func(t *testing.T) {
		// R -> X loses main and nets past the grace period, behind a younger
		// record that moves a root onto Z, so X is Dead but not yet ripe. A writer then puts X again
		// and, just inside the grace period, a parent naming it under main.
		// The put again sets the hold mark, and netting it moves X's ripe
		// sequence to the journal's end, past the parent, so no pass frees
		// X before the parent's up pair nets.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		z := tr.add(t, "Z")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "R", "Z"), []block.NamedRoot{{Name: "main", Ref: r}, {Name: "z", Ref: z}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "y", Ref: z}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(2 * time.Second)
		if res := collectAll(t, v); res.Freed != 0 {
			t.Fatalf("freed %d before the younger record netted", res.Freed)
		}

		// Put X again, then the parent just inside the grace period.
		if _, err := v.PutBlockBatch(ctx, tr.entries("X")); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		p := tr.add(t, "P", "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}

		// Netting the put again frees R only; netting the parent keeps X.
		clk.Add(2 * time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "X", "P")
		clk.Add(block_gc.DefaultGrace)
		collectAll(t, v)
		requireStored(t, v, tr, true, "X", "P", "Z")
		requireStored(t, v, tr, false, "R")
		requireAudit(t, v, 0)
	})

	t.Run("proof of a released block", func(t *testing.T) {
		// R -> C is proved complete, then loses main and nets behind a
		// younger record, so R is Dead and stored while C may go first. Its
		// proof goes with the release, and a proof marked while R is Dead
		// counts for nothing, so a copy neither skips nor holds R's tree on
		// a proof C may no longer back.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		r := tr.add(t, "R", "C")
		z := tr.add(t, "Z")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C", "R", "Z"), []block.NamedRoot{{Name: "main", Ref: r}, {Name: "z", Ref: z}}); err != nil {
			t.Fatal(err)
		}
		if err := v.MarkRootsComplete(ctx, []*block.BlockRef{r, z}); err != nil {
			t.Fatal(err)
		}
		proved := func() bool {
			t.Helper()
			complete, err := v.RootsComplete(ctx, []*block.BlockRef{r})
			if err != nil {
				t.Fatal(err)
			}
			return complete[0]
		}
		if !proved() {
			t.Fatal("R not proved")
		}

		// Release R behind a younger record.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "y", Ref: z}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(2 * time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "C")
		if proved() {
			t.Fatal("released R still proved")
		}

		// A pending proof of R counts for nothing, nor holds it under a stage,
		// and carrying it writes none.
		if err := v.MarkRootsComplete(ctx, []*block.BlockRef{r}); err != nil {
			t.Fatal(err)
		}
		if proved() {
			t.Fatal("pending proof of a released R counted")
		}
		stage, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		held, err := v.HoldStageCompleteRoots(ctx, stage, []*block.BlockRef{r})
		release()
		if err != nil {
			t.Fatal(err)
		}
		if held[0] {
			t.Fatal("stage held a released R on its proof")
		}
		if err := v.MarkRootsComplete(ctx, []*block.BlockRef{r, z}); err != nil {
			t.Fatal(err)
		}
		if proved() {
			t.Fatal("carried a proof of a released R")
		}

		// Once R ripens, it and C go.
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R", "C")
		requireStored(t, v, tr, true, "Z")
		requireAudit(t, v, 0)
	})

	t.Run("root moved onto a Dead block", func(t *testing.T) {
		// R -> E loses main and nets behind a younger record that moves a
		// root onto Z, so R and E are Dead but not yet ripe. Root k then moves onto R without a put.
		// The move sets the hold mark, so no pass frees R or E once they
		// ripen, before the move nets and re-holds them.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E")
		r := tr.add(t, "R", "E")
		z := tr.add(t, "Z")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("E", "R", "Z"), []block.NamedRoot{{Name: "main", Ref: r}, {Name: "z", Ref: z}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "y", Ref: z}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(2 * time.Second)
		collectAll(t, v)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "k", Ref: r}}); err != nil {
			t.Fatal(err)
		}

		// Once the younger record nets, R and E are ripe, and the hold mark
		// still keeps them; once the move nets, they are held again.
		clk.Add(block_gc.DefaultGrace - time.Second)
		if res := collectAll(t, v); res.Freed != 0 {
			t.Fatalf("freed %d past the hold mark", res.Freed)
		}
		clk.Add(2 * time.Second)
		collectAll(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "E", "Z")
		requireAudit(t, v, 0)
	})

	t.Run("shared child in one batch", func(t *testing.T) {
		// R -> {A, B}, A -> X, B -> X are born in one batch, and side holds
		// A too. X has two parents born beside it, so it is counted twice.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		a := tr.add(t, "A", "X")
		tr.add(t, "B", "X")
		r := tr.add(t, "R", "A", "B")
		roots := []block.NamedRoot{{Name: "main", Ref: r}, {Name: "side", Ref: a}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "A", "B", "R"), roots); err != nil {
			t.Fatal(err)
		}

		// Releasing main frees R and B; A and X stay under side.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R", "B")
		requireStored(t, v, tr, true, "A", "X")
		requireAudit(t, v, 0)

		// Releasing side frees the rest.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "side", Previous: a}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "A", "X")
	})

	t.Run("stage", func(t *testing.T) {
		// A stage builds C, then P -> C; main adopts P, and the stage's
		// release leaves P's tree to main. A second build the stage releases
		// unadopted goes after the grace period.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		stage, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tr := testTree{}
		tr.add(t, "C")
		p := tr.add(t, "P", "C")
		tr.add(t, "Q")
		for _, name := range []string{"C", "P", "Q"} {
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries(name)); err != nil {
				t.Fatal(err)
			}
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}

		// Inside the stage's life nothing is freed; after its release and
		// the grace period only the unadopted build goes.
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "C", "P", "Q")
		release()
		if err := v.withDirectAtomic(ctx, nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "C", "P")
		requireStored(t, v, tr, false, "Q")
		requireAudit(t, v, 0)
	})

	t.Run("underflow in one free slice", func(t *testing.T) {
		// A -> B under main, with B also at zero as if a holder had vanished
		// from its count. Releasing main brings A to zero, and its cascade
		// takes B below zero: the slice keeps B with one holder as an
		// underflow, and the free that follows must not free B.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		for i := 0; ; i++ {
			child := "U" + strconv.Itoa(i)
			tr.add(t, child)
			tr.add(t, "A", child)
			if bytes.Compare(block_gc.BlockKey(tr["A"].Ref), block_gc.BlockKey(tr[child].Ref)) < 0 {
				tr["B"] = tr[child]
				break
			}
		}
		a := tr["A"].Ref
		if err := v.SetBucketRoots(ctx, "b", tr.entries("B", "A"), []block.NamedRoot{{Name: "main", Ref: a}}); err != nil {
			t.Fatal(err)
		}
		err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
			return p.rec.SetHolders(ctx, tr["B"].Ref, 0, 0)
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: a}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Underflows != 1 {
			t.Fatalf("collection found %d underflows, want 1", res.Underflows)
		}
		requireStored(t, v, tr, false, "A")
		requireStored(t, v, tr, true, "B")
	})

	t.Run("read fills a tree main holds", func(t *testing.T) {
		// D is stored under old. A read fills B, which names C, which names
		// D; main moves onto B without putting its tree; a later read fills
		// C; old is released. D keeps the holder C's fill gave it, and the
		// fills are garbage only once nothing holds them.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		d := tr.add(t, "D")
		tr.add(t, "C", "D")
		b := tr.add(t, "B", "C")
		fill := func(name string) {
			t.Helper()
			err := v.withDirectFill(ctx, func(p *physicalTx) error {
				_, err := block_gc.LandFill(ctx, p.rec, p.blocks, v.now(), tr.entries(name))
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("D"), []block.NamedRoot{{Name: "old", Ref: d}}); err != nil {
			t.Fatal(err)
		}
		fill("B")
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: b}}); err != nil {
			t.Fatal(err)
		}
		fill("C")
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "old", Previous: d}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "B", "C", "D")
		requireAudit(t, v, 0)

		// Releasing main frees the whole tree.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: b}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "B", "C", "D")
	})

	t.Run("unheld fill", func(t *testing.T) {
		// A read fills E, which nothing holds: it stays for the grace period
		// and then goes, as an unheld put does.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E")
		err := v.withDirectFill(ctx, func(p *physicalTx) error {
			_, err := block_gc.LandFill(ctx, p.rec, p.blocks, v.now(), tr.entries("E"))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		collectAll(t, v)
		requireStored(t, v, tr, true, "E")
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "E")
	})

	t.Run("stage lease lapses while open", func(t *testing.T) {
		// A stage holds T, and its process stalls past the lease: a collector
		// that does not renew, as another process's does, releases the
		// stage's root. The next renewal holds it again, so after the grace
		// period a parent the stage's writer puts still finds T.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		stage, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tr := testTree{}
		tr.add(t, "T")
		if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("T")); err != nil {
			t.Fatal(err)
		}
		clk.Add(rootLease + time.Second)
		if res, err := v.collector.Collect(ctx); err != nil || res.Lapsed != 1 {
			t.Fatalf("stalled pass lapsed %d err %v, want the stage root", res.Lapsed, err)
		}
		if _, err := v.renewRoots(ctx); err != nil {
			t.Fatal(err)
		}
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "T")

		// The parent names T under main, the stage lets it go, and T stays.
		p := tr.add(t, "P", "T")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		release()
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "T")
		requireAudit(t, v, 0)
	})

	for _, renew := range []bool{false, true} {
		t.Run("stage stalled past the grace period renew="+strconv.FormatBool(renew), func(t *testing.T) {
			// A stage holds T, and its process stalls past the lease and
			// the grace period: a collector that does not renew releases
			// the stage's root and frees T. The stage's next put names T,
			// and it is refused, as is any put after a renewal that finds T
			// gone, rather than landing S over a freed block.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			stage, release, err := v.OpenStage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			tr := testTree{}
			tr.add(t, "T")
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("T")); err != nil {
				t.Fatal(err)
			}
			clk.Add(rootLease + time.Second)
			if _, err := v.collector.Collect(ctx); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			for range 3 {
				if _, err := v.collector.Collect(ctx); err != nil {
					t.Fatal(err)
				}
			}
			requireStored(t, v, tr, false, "T")
			if renew {
				if _, err := v.renewRoots(ctx); err != nil {
					t.Fatal(err)
				}
			}
			tr.add(t, "S", "T")
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("S")); !errors.Is(err, block.ErrStageReleased) {
				t.Fatalf("put over a freed stage top = %v, want the stage lost", err)
			}
			requireStored(t, v, tr, false, "S")
		})
	}

	for _, audited := range []bool{true, false} {
		t.Run("hole refilled by a stage audited="+strconv.FormatBool(audited), func(t *testing.T) {
			// main -> R -> H, and the store loses H with its child list. A
			// stage then puts H again and drops it. Once an audit recorded
			// the hole, the put counts H as stored under R, so H stays.
			// Refilled before any audit, H counts as a new block under the
			// stage alone, and the audit reports the undercount.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			h := tr.add(t, "H")
			r := tr.add(t, "R", "H")
			if err := v.SetBucketRoots(ctx, "b", tr.entries("H", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
				t.Fatal(err)
			}
			punchHole(t, v, h)
			if audited {
				if res, err := v.Audit(ctx); err != nil || res.Holes != 1 {
					t.Fatalf("audit recorded %d holes err %v, want 1", res.Holes, err)
				}
			}
			stage, release, err := v.OpenStage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("H")); err != nil {
				t.Fatal(err)
			}
			release()
			if err := v.withDirectAtomic(ctx, nil); err != nil {
				t.Fatal(err)
			}
			if !audited {
				if res, err := v.Audit(ctx); err != nil || res.Undercount != 1 {
					t.Fatalf("audit of an unrecorded refill found %d undercounts err %v, want 1", res.Undercount, err)
				}
				return
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
			requireStored(t, v, tr, true, "R", "H")
			requireAudit(t, v, 0)
		})
	}

	t.Run("cache fill during an adoption", func(t *testing.T) {
		// main holds C on an unadopted Volume. While an adoption settles its
		// counts, after it counted the child lists, a cache fill stores F
		// naming C, with a fill record that holds C and takes F's implied
		// holder back. The adoption treats F as born and leaves it to its
		// record. A root r then moves onto F: whichever of the journal and
		// the fill record nets first, F stays while r holds it, and once r is
		// released F goes and C stays under main.
		store := store_kvtx_inmem.NewStore()
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		c := tr.add(t, "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "main", Ref: c}}); err != nil {
			t.Fatal(err)
		}
		f := tr.add(t, "F", "C")
		probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptSettle: func() {
				if _, _, err := v.PrepareOwnedBlock(ctx, "", tr["F"].Data, &block.PutOpts{Refs: tr["F"].Refs, RefsKnown: true, CacheFill: true}); err != nil {
					t.Error(err)
				}
			},
		}}
		if _, err := block_gc.Adopt(ctx, probe); err != nil || len(probe.at) != 0 {
			t.Fatalf("adopt = %v with %d fills left", err, len(probe.at))
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "r", Ref: f}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "C", "F")
		requireAudit(t, v, 0)

		// Released, F goes and C stays.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "r", Previous: f}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "F")
		requireStored(t, v, tr, true, "C")
		requireAudit(t, v, 0)
	})

	// main holds P naming B, which the Volume lacks, on an unadopted Volume,
	// and a read fills B at a point of an adoption: while it counts the
	// child lists, or between its clear and its mark. Either way B keeps
	// P's holder after the grace period. The hooks also check the probe's
	// positions against the adoption state.
	for _, at := range []struct {
		name  string
		pos   int
		state block_gc.Adoption
	}{
		{"during the list scan", adoptCountChildren, block_gc.Adoption_ADOPTION_ADOPTING},
		{"between the clear and the mark", adoptMark, block_gc.Adoption_ADOPTION_UNADOPTED},
	} {
		t.Run("cache fill "+at.name, func(t *testing.T) {
			// Store P naming the missing B under main.
			store := store_kvtx_inmem.NewStore()
			setUnadopted(t, store)
			v, clk := openUnadoptedVolume(t, store)
			ctx := t.Context()
			tr := testTree{}
			tr.add(t, "B")
			p := tr.add(t, "P", "B")
			if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
				t.Fatal(err)
			}

			// Adopt, filling B at the point.
			probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
				at.pos: func() {
					if got := livenessAdoption(t, v); got != at.state {
						t.Errorf("adoption at position %d is %v, want %v", at.pos, got, at.state)
					}
					err := v.withDirectFill(ctx, func(px *physicalTx) error {
						_, err := block_gc.LandFill(ctx, px.rec, px.blocks, v.now(), tr.entries("B"))
						return err
					})
					if err != nil {
						t.Error(err)
					}
				},
			}}
			if _, err := block_gc.Adopt(ctx, probe); err != nil || len(probe.at) != 0 {
				t.Fatalf("adopt = %v with %d fills left", err, len(probe.at))
			}

			// B keeps P's holder, and goes once main lets P go.
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
			requireStored(t, v, tr, true, "P", "B")
			requireAudit(t, v, 0)
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: p}}); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
			requireStored(t, v, tr, false, "P", "B")
		})
	}

	// A Volume reopens with fill records not netted yet: adopted, they net
	// after the reopen; unadopted with frees on, the background adoption
	// clears them and counts the lists. Either way B keeps P's holder, and
	// the unheld fill G goes.
	for _, adopted := range []bool{true, false} {
		t.Run("reopen with fill records adopted="+strconv.FormatBool(adopted), func(t *testing.T) {
			// Store P naming the missing B under main, then fill B and G.
			store := store_kvtx_inmem.NewStore()
			if !adopted {
				setUnadopted(t, store)
			}
			v1, _ := openUnadoptedVolume(t, store)
			ctx := t.Context()
			tr := testTree{}
			tr.add(t, "B")
			tr.add(t, "G")
			p := tr.add(t, "P", "B")
			if err := v1.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
				t.Fatal(err)
			}
			err := v1.withDirectFill(ctx, func(px *physicalTx) error {
				_, err := block_gc.LandFill(ctx, px.rec, px.blocks, v1.now(), tr.entries("B", "G"))
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := v1.Close(); err != nil {
				t.Fatal(err)
			}

			// Reopen with frees on, wait for any adoption, and collect.
			v, clk := openLivenessVolume(t, store, true)
			waitAdopted(t, v)
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
			requireStored(t, v, tr, true, "P", "B")
			requireStored(t, v, tr, false, "G")
			requireAudit(t, v, 0)
		})
	}

	t.Run("background adoption", func(t *testing.T) {
		// An unadopted Volume holding main -> R -> X and an unheld G opens
		// with frees on and adopts itself in the background: R and X stay
		// and G goes after the grace period.
		store := store_kvtx_inmem.NewStore()
		setUnadopted(t, store)
		v1, _ := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		tr.add(t, "G")
		if err := v1.SetBucketRoots(ctx, "b", tr.entries("X", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		if _, err := v1.PutBlockBatch(ctx, tr.entries("G")); err != nil {
			t.Fatal(err)
		}
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}

		// Reopen with frees on and wait for the adoption.
		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")
		requireStored(t, v, tr, false, "G")
		requireAudit(t, v, 0)

		// A list rebuild that rewrites a list of the adopted Volume returns it
		// to unadopted, and the background adoption recounts it without a
		// reopen: N, stored unheld and then named in R's rewritten list,
		// stays.
		setListsUnverified(t, v, true)
		n := tr.add(t, "N")
		if _, err := v.PutBlockBatch(ctx, tr.entries("N")); err != nil {
			t.Fatal(err)
		}
		rb, err := v.OpenListRebuild(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := rb.Verify(ctx, tr["X"].Ref, nil); err != nil {
			t.Fatal(err)
		}
		if err := rb.Verify(ctx, r, []*block.BlockRef{tr["X"].Ref, n}); err != nil {
			t.Fatal(err)
		}
		if res, err := rb.Finish(ctx); err != nil || res.Rewritten != 1 || !res.Cleared || !res.Unadopted {
			t.Fatalf("rebuild with a rewrite = %+v err %v, want the Volume unadopted", res, err)
		}
		waitAdopted(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X", "N")
		requireAudit(t, v, 0)
	})

	t.Run("cache fill of a held block during an adoption", func(t *testing.T) {
		// main holds P on an unadopted Volume, and P names B, which the
		// Volume lacks. While an adoption runs, before it settles, a read
		// fills B. The adoption counts P's holder of B and leaves B to its
		// fill record, so after the grace period B keeps P's holder.
		store := store_kvtx_inmem.NewStore()
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "B")
		p := tr.add(t, "P", "B")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptSettle: func() {
				err := v.withDirectFill(ctx, func(px *physicalTx) error {
					_, err := block_gc.LandFill(ctx, px.rec, px.blocks, v.now(), tr.entries("B"))
					return err
				})
				if err != nil {
					t.Error(err)
				}
			},
		}}
		if _, err := block_gc.Adopt(ctx, probe); err != nil || len(probe.at) != 0 {
			t.Fatalf("adopt = %v with %d fills left", err, len(probe.at))
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "B")
		requireAudit(t, v, 0)
	})

	t.Run("record larger than a slice", func(t *testing.T) {
		// A under root a names 5000 leaves. B, born under root b, names the
		// same stored leaves, so its one record carries more up pairs than a
		// net slice applies and splits across slices. Releasing a frees only
		// A; releasing b frees B and every leaf.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		leaves := make([]string, 5000)
		for i := range leaves {
			leaves[i] = "leaf " + strconv.Itoa(i)
			tr.add(t, leaves[i])
		}
		a := tr.add(t, "A", leaves...)
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "A")...), []block.NamedRoot{{Name: "a", Ref: a}}); err != nil {
			t.Fatal(err)
		}
		b := tr.add(t, "B", leaves...)
		if err := v.SetBucketRoots(ctx, "b", tr.entries("B"), []block.NamedRoot{{Name: "b", Ref: b}, {Name: "a", Previous: a}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "A")
		requireStored(t, v, tr, true, append(slices.Clone(leaves), "B")...)
		requireAudit(t, v, 0)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "b", Previous: b}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, append(slices.Clone(leaves), "B")...)
		requireAudit(t, v, 0)
	})

	t.Run("record larger than a slice", func(t *testing.T) {
		// 20,000 leaves stay held under mids of 1000, each under its root.
		// One commit stores a parent naming each mid's leaves, so its record
		// carries an up pair for every leaf, more than a net slice holds.
		// The commit writes it in pages and the collector nets it across
		// slices once it ages, so no collector transaction does as much work
		// as the record has pairs or reads half of its bytes.
		ctx := t.Context()
		cs := kvtx_kvtest.NewCountingStore(store_kvtx_inmem.NewStore())
		v, err := NewVolume(ctx, "test/wide", store_kvkey.NewDefaultKVKey(), cs, &store_kvtx.Config{CollectorFrees: true}, false, false, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		clk := &testClock{now: time.Unix(1_800_000_000, 0)}
		v.clock = clk.Now
		t.Cleanup(func() { _ = v.Close() })
		tr := testTree{}
		const pairs = 20_000
		var parents []string
		for m := 0; m*1000 < pairs; m++ {
			names := make([]string, 0, 1000)
			for i := m * 1000; i < (m+1)*1000; i++ {
				names = append(names, "leaf "+strconv.Itoa(i))
				tr.add(t, names[len(names)-1])
			}
			mid := "mid " + strconv.Itoa(m)
			ref := tr.add(t, mid, names...)
			if err := v.SetBucketRoots(ctx, "b", tr.entries(append(names, mid)...), []block.NamedRoot{{Name: mid, Ref: ref}}); err != nil {
				t.Fatal(err)
			}
			parents = append(parents, "parent "+strconv.Itoa(m))
			tr.add(t, parents[m], names...)
		}
		top := tr.add(t, "top", parents...)
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(parents), "top")...), []block.NamedRoot{{Name: "top", Ref: top}}); err != nil {
			t.Fatal(err)
		}
		collect := func() {
			t.Helper()
			cs.ResetMax()
			collectAll(t, v)
			if m := cs.Counts().MaxWriteOps; m >= pairs {
				t.Fatalf("a collector transaction made %d operations over a record of %d pairs", m, pairs)
			}
			if b := cs.Counts().MaxWriteReadBytes; b >= pairs*33/2 {
				t.Fatalf("a collector transaction read %d bytes of a record of %d pairs", b, pairs)
			}
		}

		// Leave the young record, then net it once it ages.
		collect()
		clk.Add(block_gc.DefaultGrace + time.Second)
		collect()
		requireStored(t, v, tr, true, append(slices.Clone(parents), "top", "leaf 0", "leaf 19999")...)
		requireAudit(t, v, 0)
	})

	t.Run("commit slower than a slice", func(t *testing.T) {
		// W names 2,000 leaves under root w, released past the grace period.
		// Each transaction's commit costs 3 ms the collector's own work does
		// not, on the wall clock and on the Volume's, more than a slice aims
		// to hold the writer. The pace counts only the collector's work, so
		// the collection frees W and its leaves in a bounded number of
		// transactions rather than shrinking every slice to its smallest.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		leaves := make([]string, 2000)
		for i := range leaves {
			leaves[i] = "leaf " + strconv.Itoa(i)
			tr.add(t, leaves[i])
		}
		w := tr.add(t, "W", leaves...)
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "W")...), []block.NamedRoot{{Name: "w", Ref: w}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "w", Previous: w}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		probe := &paceProbe{Store: collectorStore{v: v}, limit: 100, after: func() {
			clk.Add(3 * time.Millisecond)
			time.Sleep(3 * time.Millisecond)
		}}
		if freed := collectThrough(t, probe, block_gc.CollectorOptions{Frees: true, Now: v.now}); freed != 2001 {
			t.Fatalf("freed %d of W and its 2000 leaves", freed)
		}
		requireStored(t, v, tr, false, append(slices.Clone(leaves), "W")...)
	})

	t.Run("writer always waiting", func(t *testing.T) {
		// G names 100 leaves under root g, released past the grace period,
		// and H names 64 others under root h. A writer waits on every
		// collector transaction and after each commits a new root on a parent
		// of H's leaves, an up pair for each. A waiting writer does not
		// shrink the netting slices, so they reach the journal's end. G
		// ripens at the journal's end its release netted at, behind the
		// writer's first records, and once those age, G and its leaves are
		// freed in a bounded number of transactions while the writer's
		// blocks stay.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		leaves := make([]string, 100)
		for i := range leaves {
			leaves[i] = "leaf " + strconv.Itoa(i)
			tr.add(t, leaves[i])
		}
		g := tr.add(t, "G", leaves...)
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "G")...), []block.NamedRoot{{Name: "g", Ref: g}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "g", Previous: g}}); err != nil {
			t.Fatal(err)
		}
		shared := make([]string, 64)
		for i := range shared {
			shared[i] = "shared " + strconv.Itoa(i)
			tr.add(t, shared[i])
		}
		h := tr.add(t, "H", shared...)
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(shared), "H")...), []block.NamedRoot{{Name: "h", Ref: h}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		held := append(slices.Clone(shared), "H")
		probe := &paceProbe{Store: collectorStore{v: v}, limit: 1000, after: func() {
			root := "round " + strconv.Itoa(len(held))
			ref := tr.add(t, root, shared...)
			if err := v.SetBucketRoots(ctx, "b", tr.entries(root), []block.NamedRoot{{Name: root, Ref: ref}}); err != nil {
				t.Fatal(err)
			}
			held = append(held, root)
		}}
		opts := block_gc.CollectorOptions{
			Frees:             true,
			Now:               v.now,
			ForegroundWaiting: func() bool { return true },
		}
		if freed := collectThrough(t, probe, opts); freed != 0 {
			t.Fatalf("freed %d before the writer's records aged", freed)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if freed := collectThrough(t, probe, opts); freed != 101 {
			t.Fatalf("freed %d of G and its 100 leaves", freed)
		}
		requireStored(t, v, tr, false, append(slices.Clone(leaves), "G")...)
		requireStored(t, v, tr, true, held...)
		requireAudit(t, v, 0)
	})

	for _, mid := range []string{"a crash", "children held again", "a second parent's release", "the releasing block put again", "a crash mid re-hold", "the releasing block lost"} {
		t.Run("release interrupted by "+mid, func(t *testing.T) {
			// F names 6,000 leaves, more than one slice moves, and G names
			// the first and the last of them in F's list. Root f releases
			// F, and the collector stops once a transaction left F's paged
			// release in progress, with the first leaves released and the
			// last not, and F Dying, stored until its release ends.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			leaves := make([]string, 6_000)
			for i := range leaves {
				leaves[i] = "leaf " + strconv.Itoa(i)
				tr.add(t, leaves[i])
			}
			slices.SortFunc(leaves, func(a, b string) int {
				return bytes.Compare(block_gc.BlockKey(tr[a].Ref), block_gc.BlockKey(tr[b].Ref))
			})
			first, last := leaves[0], leaves[len(leaves)-1]
			f := tr.add(t, "F", leaves...)
			g := tr.add(t, "G", first, last)
			roots := []block.NamedRoot{{Name: "f", Ref: f}, {Name: "g", Ref: g}}
			if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "F", "G")...), roots); err != nil {
				t.Fatal(err)
			}
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "f", Previous: f}}); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			probe := &stopMidRelease{Store: collectorStore{v: v}, v: v}
			c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
			if _, err := c.Collect(ctx); !errors.Is(err, errMidRelease) {
				t.Fatalf("collect = %v, want it stopped mid-release", err)
			}
			requireStored(t, v, tr, true, "F")

			// Act while the release is in progress, then collect to the end
			// and once more after every record ages.
			kept := []string{"G", first, last}
			switch mid {
			case "children held again":
				err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "k1", Ref: tr[leaves[1]].Ref}, {Name: "k2", Ref: tr[leaves[len(leaves)-2]].Ref}})
				if err != nil {
					t.Fatal(err)
				}
				kept = append(kept, leaves[1], leaves[len(leaves)-2])
			case "a second parent's release":
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "g", Previous: g}}); err != nil {
					t.Fatal(err)
				}
				kept = nil
			case "the releasing block put again", "a crash mid re-hold":
				if err := v.SetBucketRoots(ctx, "b", tr.entries("F"), []block.NamedRoot{{Name: "f", Ref: f}}); err != nil {
					t.Fatal(err)
				}
				kept = append(slices.Clone(leaves), "F", "G")
			case "the releasing block lost":
				punchHole(t, v, f)
			}
			res := collectAll(t, v)
			clk.Add(block_gc.DefaultGrace + time.Second)

			// Stop again once the put again's paged re-hold is in progress.
			if mid == "a crash mid re-hold" {
				probe := &stopMidRelease{Store: collectorStore{v: v}, v: v, rehold: true}
				c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
				if _, err := c.Collect(ctx); !errors.Is(err, errMidRelease) {
					t.Fatalf("collect = %v, want it stopped mid re-hold", err)
				}
			}
			if more := collectAll(t, v); res.Underflows+more.Underflows != 0 {
				t.Fatalf("%d underflows releasing F", res.Underflows+more.Underflows)
			}

			// Exactly the blocks still held stay, and the counts are exact.
			gone := make([]string, 0, len(leaves))
			for _, name := range append(slices.Clone(leaves), "F", "G") {
				if !slices.Contains(kept, name) {
					gone = append(gone, name)
				}
			}
			requireStored(t, v, tr, true, kept...)
			requireStored(t, v, tr, false, gone...)
			requireAudit(t, v, 0)
		})
	}

	t.Run("hole under a release in progress", func(t *testing.T) {
		// F names 6,000 leaves, and P1 and P2 name its last, H. Root f
		// releases F, and the collector stops with F's paged release not yet
		// at H. The audit then counts the release among H's holders and
		// leaks none of the leaves it has not reached. The store loses
		// H, and the audit records the hole with F's holder in its count, so
		// once the release reaches H, H is put again and P1 is freed, P2
		// still holds H.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		leaves := make([]string, 6_000)
		for i := range leaves {
			leaves[i] = "leaf " + strconv.Itoa(i)
			tr.add(t, leaves[i])
		}
		slices.SortFunc(leaves, func(a, b string) int {
			return bytes.Compare(block_gc.BlockKey(tr[a].Ref), block_gc.BlockKey(tr[b].Ref))
		})
		h := leaves[len(leaves)-1]
		f := tr.add(t, "F", leaves...)
		p1 := tr.add(t, "P1", h)
		p2 := tr.add(t, "P2", h)
		roots := []block.NamedRoot{{Name: "f", Ref: f}, {Name: "p1", Ref: p1}, {Name: "p2", Ref: p2}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "F", "P1", "P2")...), roots); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "f", Previous: f}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		probe := &stopMidRelease{Store: collectorStore{v: v}, v: v}
		c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
		if _, err := c.Collect(ctx); !errors.Is(err, errMidRelease) {
			t.Fatalf("collect = %v, want it stopped mid-release", err)
		}
		requireStored(t, v, tr, true, "F", h)
		requireAudit(t, v, 0)

		// Lose H and audit, finish the release, and put H again.
		punchHole(t, v, tr[h].Ref)
		if res, err := v.Audit(ctx); err != nil || res.Holes != 1 {
			t.Fatalf("audit recorded %d holes err %v, want 1", res.Holes, err)
		}
		collectAll(t, v)
		if err := v.SetBucketRoots(ctx, "b", tr.entries(h), nil); err != nil {
			t.Fatal(err)
		}

		// Free P1: P2 still holds H.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "p1", Previous: p1}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P2", h)
		requireStored(t, v, tr, false, "F", "P1", leaves[0])
		requireAudit(t, v, 0)
	})

	t.Run("hole under a deferred release", func(t *testing.T) {
		// D1 names E1 and D2 names 4,000 leaves, and roots d1 and d2 let
		// them go in records one slice nets. D1's release moves E1 first,
		// so D2's no longer fits the slice and its cascade record stays
		// due. The store then loses D2 with its list. The next pass drops
		// the release that can never run, its leaves keeping D2's holder as
		// leaked garbage, and the frees go on: G, let go after, is freed.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E1")
		d1 := tr.add(t, "D1", "E1")
		leaves := make([]string, 4_000)
		for i := range leaves {
			leaves[i] = "leaf " + strconv.Itoa(i)
			tr.add(t, leaves[i])
		}
		d2 := tr.add(t, "D2", leaves...)
		g := tr.add(t, "G")
		roots := []block.NamedRoot{{Name: "d1", Ref: d1}, {Name: "d2", Ref: d2}, {Name: "g", Ref: g}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "E1", "D1", "D2", "G")...), roots); err != nil {
			t.Fatal(err)
		}
		for _, root := range []block.NamedRoot{{Name: "d1", Previous: d1}, {Name: "d2", Previous: d2}} {
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{root}); err != nil {
				t.Fatal(err)
			}
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		probe := &stopMidRelease{Store: collectorStore{v: v}, v: v, cascade: true}
		c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
		if _, err := c.Collect(ctx); !errors.Is(err, errMidRelease) || livenessCascades(t, v) != 1 {
			t.Fatalf("collect = %v with %d cascades due, want it stopped with D2's", err, livenessCascades(t, v))
		}

		// Lose D2, let G go, and collect past the grace period.
		punchHole(t, v, d2)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "g", Previous: g}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		res := collectAll(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		more := collectAll(t, v)
		if res.Stranded+more.Stranded != 1 || livenessCascades(t, v) != 0 {
			t.Fatalf("dropped %d releases with %d due, want D2's dropped", res.Stranded+more.Stranded, livenessCascades(t, v))
		}
		requireStored(t, v, tr, false, "D1", "E1", "G")
		requireStored(t, v, tr, true, leaves[0], leaves[len(leaves)-1])
		requireAudit(t, v, uint64(len(leaves)))
	})

	t.Run("hole under a deferred re-hold", func(t *testing.T) {
		// H names 3,072 leaves, as many as a slice moves, and root h lets
		// it go, so H and its leaves are Dead. Root h2 holds H again in a
		// record one slice nets with root d1's release of D1, which names
		// E1: D1's release moves E1 first, so H's re-hold no longer fits
		// and stays due. The store then loses H with its list while its
		// leaves are Dead and ripe. Losing a Holding block is outside the
		// contract, and the frees wait on the re-hold rather than free the
		// leaves under root h2; the audit reports the record, and once a
		// put refills H the re-hold runs and the leaves stay.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E1")
		d1 := tr.add(t, "D1", "E1")
		leaves := make([]string, 3_072)
		for i := range leaves {
			leaves[i] = "leaf " + strconv.Itoa(i)
			tr.add(t, leaves[i])
		}
		h := tr.add(t, "H", leaves...)
		roots := []block.NamedRoot{{Name: "d1", Ref: d1}, {Name: "h", Ref: h}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "E1", "D1", "H")...), roots); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "h", Previous: h}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		nets := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{Now: v.now})
		for more := true; more; {
			res, err := nets.Collect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			more = res.More
		}

		// Hold H again as D1 goes, and stop with H's re-hold due.
		for _, root := range []block.NamedRoot{{Name: "h2", Ref: h}, {Name: "d1", Previous: d1}} {
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{root}); err != nil {
				t.Fatal(err)
			}
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		probe := &stopMidRelease{Store: collectorStore{v: v}, v: v, cascade: true}
		c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
		if _, err := c.Collect(ctx); !errors.Is(err, errMidRelease) || livenessCascades(t, v) != 1 {
			t.Fatalf("collect = %v with %d cascades due, want it stopped with H's re-hold", err, livenessCascades(t, v))
		}

		// Lose H: the frees wait, and the audit reports the record.
		punchHole(t, v, h)
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 0 || res.Stranded != 0 {
			t.Fatalf("freed %d and dropped %d under H's due re-hold", res.Freed, res.Stranded)
		}
		requireStored(t, v, tr, true, append(slices.Clone(leaves), "D1", "E1")...)
		if res, err := v.Audit(ctx); err != nil || res.UnlistedCascades != 1 || res.Holes != 1 {
			t.Fatalf("audit found %d unlisted cascades and %d holes err %v, want 1 and 1", res.UnlistedCascades, res.Holes, err)
		}

		// Refill H: the re-hold runs, and only D1 and E1 go.
		if err := v.SetBucketRoots(ctx, "b", tr.entries("H"), nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, append(slices.Clone(leaves), "H")...)
		requireStored(t, v, tr, false, "D1", "E1")
		requireAudit(t, v, 0)
	})

	t.Run("fill lands between the netting and the free", func(t *testing.T) {
		// Root c lets C go. Once a pass nets that, and before it frees, a
		// read fills P, which names C. The free waits for the fill's up,
		// so a root that moves onto P after the grace period finds C.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		cref := tr.add(t, "C")
		p := tr.add(t, "P", "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "c", Ref: cref}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "c", Previous: cref}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		filled := false
		fill := func(p block_gc.Phase) func() {
			if p == block_gc.PhaseSweep && !filled {
				filled = true
				err := v.withDirectFill(ctx, func(px *physicalTx) error {
					_, err := block_gc.LandFill(ctx, px.rec, px.blocks, v.now(), tr.entries("P"))
					return err
				})
				if err != nil {
					t.Error(err)
				}
			}
			return func() {}
		}
		c := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{Frees: true, Now: v.now, Phase: fill})
		if res, err := c.Collect(ctx); err != nil || !filled || res.Freed != 0 {
			t.Fatalf("collect freed %d err %v filled %v, want P filled and C kept", res.Freed, err, filled)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "C")
		requireAudit(t, v, 0)
	})

	for _, landed := range []bool{true, false} {
		t.Run("holder landed after an audit's snapshot landed="+strconv.FormatBool(landed), func(t *testing.T) {
			// P1 and P2 name H under roots p1 and p2, and the store loses H.
			// An audit tallies P1 and P2 in its snapshot. With landed set, the
			// counts hold both, and before the audit records the hole a
			// writer lands P3 naming H, which the collector nets. Otherwise
			// the up pair of H's second holder is still in the journal at the
			// snapshot and nothing lands. Either way the audit records H with
			// its true holders: once H is put again and P1 and P2 are freed,
			// P3 or nothing holds H, and H stays while P3 does and is freed
			// with it.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			h := tr.add(t, "H")
			p1 := tr.add(t, "P1", "H")
			p2 := tr.add(t, "P2", "H")
			p3 := tr.add(t, "P3", "H")
			roots := []block.NamedRoot{{Name: "p1", Ref: p1}, {Name: "p2", Ref: p2}}
			if err := v.SetBucketRoots(ctx, "b", tr.entries("H", "P1", "P2"), roots); err != nil {
				t.Fatal(err)
			}
			if landed {
				clk.Add(block_gc.DefaultGrace + time.Second)
				collectAll(t, v)
			}
			punchHole(t, v, h)

			// Audit, landing P3 between the snapshot and the hole's record.
			probe := &auditProbe{collectorStore: collectorStore{v: v}}
			if landed {
				probe.before = func() {
					if err := v.SetBucketRoots(ctx, "b", tr.entries("P3"), []block.NamedRoot{{Name: "p3", Ref: p3}}); err != nil {
						t.Error(err)
					}
					clk.Add(block_gc.DefaultGrace + time.Second)
					collectAll(t, v)
				}
			}
			if res, err := block_gc.Audit(ctx, probe); err != nil || res.Holes != 1 {
				t.Fatalf("audit recorded %d holes err %v, want 1", res.Holes, err)
			}

			// Put H again and free P1 and P2.
			if err := v.SetBucketRoots(ctx, "b", tr.entries("H"), nil); err != nil {
				t.Fatal(err)
			}
			release := func(name string, ref *block.BlockRef) {
				t.Helper()
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: name, Previous: ref}}); err != nil {
					t.Fatal(err)
				}
				for range 2 {
					clk.Add(block_gc.DefaultGrace + time.Second)
					collectAll(t, v)
				}
			}
			release("p1", p1)
			release("p2", p2)
			requireStored(t, v, tr, false, "P1", "P2")
			if landed {
				requireStored(t, v, tr, true, "P3", "H")
				requireAudit(t, v, 0)
				release("p3", p3)
			}
			requireStored(t, v, tr, false, "P3", "H")
			requireAudit(t, v, 0)
		})
	}

	t.Run("child named twice", func(t *testing.T) {
		// P names the stored C twice and an empty ref. Its list keeps C once,
		// so releasing P frees both, and the audit finds nothing.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		c := tr.add(t, "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "c", Ref: c}}); err != nil {
			t.Fatal(err)
		}
		p := tr.add(t, "P", "C", "C")
		tr["P"].Refs = append(tr["P"].Refs, &block.BlockRef{})
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "p", Ref: p}, {Name: "c", Previous: c}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "p", Previous: p}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "P", "C")
		requireAudit(t, v, 0)
	})

	t.Run("bytes without a list", func(t *testing.T) {
		// Bytes stored below the Volume's liveness, without a child list,
		// are garbage nothing frees; the audit reports them.
		v, _ := newLivenessVolume(t, true)
		ctx := t.Context()
		if _, _, err := v.Store.PutBlock(ctx, []byte("stored without a list"), &block.PutOpts{RefsKnown: true}); err != nil {
			t.Fatal(err)
		}
		if res, err := v.Audit(ctx); err != nil || res.Unlisted != 1 {
			t.Fatalf("audit found %d unlisted blocks err %v, want 1", res.Unlisted, err)
		}
	})

	t.Run("abandoned write", func(t *testing.T) {
		// A writer puts P -> C in two batches, child first, and never lands
		// a root on them. Once the grace period passes both are garbage.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		tr.add(t, "P", "C")
		for _, name := range []string{"C", "P"} {
			if _, err := v.PutBlockBatch(ctx, tr.entries(name)); err != nil {
				t.Fatal(err)
			}
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "P", "C")
		requireAudit(t, v, 0)
	})

	t.Run("parent put before its child", func(t *testing.T) {
		// A buffered drain puts P under main before its child C, and C lands
		// alone within the grace period. Both stay, and releasing main frees
		// both.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		p := tr.add(t, "P", "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		if _, err := v.PutBlockBatch(ctx, tr.entries("C")); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "C")
		requireAudit(t, v, 0)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: p}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "P", "C")
		requireAudit(t, v, 0)
	})

	t.Run("parent collected before its child is stored", func(t *testing.T) {
		// A session puts R2 under main before its child C2, journaling an up
		// for C2. Main lets R2 go and the collector frees it; the cascade
		// takes C2's holder although C2 has no list yet. C2 then lands alone
		// and nothing holds it, so it goes after the grace period.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C2")
		r2 := tr.add(t, "R2", "C2")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r2}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R2")
		if _, err := v.PutBlockBatch(ctx, tr.entries("C2")); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "C2")
		requireAudit(t, v, 0)
	})

	for _, pinned := range []bool{true, false} {
		t.Run("long staged session over an abandoned base pinned="+strconv.FormatBool(pinned), func(t *testing.T) {
			// main holds B -> X. A staged session observes B, pinning it as
			// the World engine's StageWorldState does. Main abandons B, and
			// the session outlasts the grace period before it puts S naming
			// X without putting X again and lands S under main. With the pin
			// X stays; without it X is freed before S names it.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			tr.add(t, "X")
			b := tr.add(t, "B", "X")
			if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "B"), []block.NamedRoot{{Name: "main", Ref: b}}); err != nil {
				t.Fatal(err)
			}
			stage, release, err := v.OpenStage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if pinned {
				unpin, err := v.PinBucketRoot(ctx, b)
				if err != nil {
					t.Fatal(err)
				}
				defer unpin()
			}
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: b}}); err != nil {
				t.Fatal(err)
			}
			advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
			collectAll(t, v)
			s := tr.add(t, "S", "X")
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("S")); err != nil {
				t.Fatal(err)
			}
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: s}}); err != nil {
				t.Fatal(err)
			}
			release()
			requireStored(t, v, tr, pinned, "X")
		})
	}

	t.Run("frees off", func(t *testing.T) {
		// With frees off the collector nets and frees nothing.
		v, clk := newLivenessVolume(t, false)
		ctx := t.Context()
		tr := testTree{}
		r1 := tr.add(t, "R1")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 0 || res.Netted == 0 {
			t.Fatalf("freed %d netted %d, want 0 freed and the journal netted", res.Freed, res.Netted)
		}
		requireStored(t, v, tr, true, "R1")
	})

	t.Run("unadopted", func(t *testing.T) {
		// A Volume that predates adoption journals and frees nothing.
		store := store_kvtx_inmem.NewStore()
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E")
		r1 := tr.add(t, "R1", "E")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("E", "R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		r2 := tr.add(t, "R2")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
			t.Fatal(err)
		}

		// Pk under root pk names K, which no put has stored yet.
		k := tr.add(t, "K")
		pk := tr.add(t, "Pk", "K")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("Pk"), []block.NamedRoot{{Name: "pk", Ref: pk}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 0 || res.Netted != 0 {
			t.Fatalf("unadopted pass freed %d netted %d, want nothing", res.Freed, res.Netted)
		}
		requireStored(t, v, tr, true, "R1", "E", "R2", "Pk")

		// An adoption interrupted while it settles the counts, with main
		// moving onto a new R3 naming E while it counts, leaves the Volume
		// unadopted.
		r3 := tr.add(t, "R3", "E")
		probe := &adoptProbe{Store: collectorStore{v: v}}
		probeCtx, cancel := context.WithCancel(ctx)
		probe.at = map[int]func(){
			adoptCountRoots: func() {
				if err := v.SetBucketRoots(ctx, "b", tr.entries("R3"), []block.NamedRoot{{Name: "main", Ref: r3, Previous: r2}}); err != nil {
					t.Error(err)
				}
			},
			adoptSettle: cancel,
		}
		if _, err := block_gc.Adopt(probeCtx, probe); !errors.Is(err, context.Canceled) || len(probe.at) != 0 {
			t.Fatalf("interrupted adopt = %v with %d writes left, want canceled after both", err, len(probe.at))
		}
		if adoption := livenessAdoption(t, v); adoption != block_gc.Adoption_ADOPTION_UNADOPTED {
			t.Fatalf("interrupted adopt left %v, want unadopted", adoption)
		}
		// It leaves its counts for the next claim to clear, so a Close that
		// cancels it waits for one transaction.

		// A rerun counts every holder once while main moves onto a new R4
		// naming E before it counts the roots; a root aux moves onto R1 and
		// a put stores K under a root k before it counts the child lists; and
		// a put stores Y under a root late before it settles. The next pass
		// frees R2 and R3, and keeps Y, born after the rerun began, and K,
		// which Pk named before it began.
		r4 := tr.add(t, "R4", "E")
		y := tr.add(t, "Y")
		probe = &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptCountRoots: func() {
				if err := v.SetBucketRoots(ctx, "b", tr.entries("R4"), []block.NamedRoot{{Name: "main", Ref: r4, Previous: r3}}); err != nil {
					t.Error(err)
				}
			},
			adoptCountChildren: func() {
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "aux", Ref: r1}}); err != nil {
					t.Error(err)
				}
				if err := v.SetBucketRoots(ctx, "b", tr.entries("K"), []block.NamedRoot{{Name: "k", Ref: k}}); err != nil {
					t.Error(err)
				}
			},
			adoptSettle: func() {
				if err := v.SetBucketRoots(ctx, "b", tr.entries("Y"), []block.NamedRoot{{Name: "late", Ref: y}}); err != nil {
					t.Error(err)
				}
			},
		}}
		if _, err := block_gc.Adopt(ctx, probe); err != nil || len(probe.at) != 0 {
			t.Fatalf("rerun adopt = %v with %d writes left", err, len(probe.at))
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R2", "R3")
		requireStored(t, v, tr, true, "R1", "E", "R4", "Y", "Pk", "K")
		requireAudit(t, v, 0)

		// K keeps Pk's holder once root k lets it go.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "k", Previous: k}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "Pk", "K")
		requireAudit(t, v, 0)

		// A fresh adoption of the same Volume reaches the same counts and
		// queue.
		netted := livenessRecords(t, store, 'H', 'Q')
		markUnadopted(t, v)
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		if again := livenessRecords(t, store, 'H', 'Q'); !maps.Equal(again, netted) {
			t.Fatalf("readoption counts %v, want %v", again, netted)
		}

		// Releasing every root frees everything.
		releases := []block.NamedRoot{{Name: "main", Previous: r4}, {Name: "aux", Previous: r1}, {Name: "late", Previous: y}, {Name: "pk", Previous: pk}}
		if err := v.SetBucketRoots(ctx, "b", nil, releases); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R1", "E", "R4", "Y", "Pk", "K")
		requireAudit(t, v, 0)
	})

	t.Run("incomplete format 6 list", func(t *testing.T) {
		// A format 6 store holds R -> P, X, Y and P -> Z under a named root,
		// but R's child list lacks X and Y, as a crash between two slices of
		// the old graph's edge batch left it. The adoption counts no holder
		// of X or Y and queues both, so the generation rule alone keeps them
		// under the root: the free parks X, and Y, given a root for a while,
		// has that root's release waiting in the journal. The rebuild then runs on the
		// adopted Volume: it refuses a decoded list that drops P's child,
		// rewrites R's list, which leaves the counts short of R's holders of X
		// and Y, and returns the Volume to unadopted when it clears the mark.
		// The recount deletes the parked and queued records and frees
		// nothing the root reaches, and drops Y's release rather than netting
		// it onto the new counts.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := store_kvtx_inmem.NewStore()
		tr := testTree{}
		tr.add(t, "Z")
		p := tr.add(t, "P", "Z")
		tr.add(t, "X")
		y := tr.add(t, "Y")
		r := tr.add(t, "R", "P", "X", "Y")
		writeLegacyStore(t, store, keys, tr, "R", true, "Z", "P", "X", "Y", "R")
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, append([]byte{livenessPrefix[0], 'c'}, block_gc.BlockKey(r)...), block_gc.EncodeChildren([]*block.BlockRef{p})); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v, clk := openUnadoptedVolume(t, store)
		if res, err := v.Adopt(ctx); err != nil || res.Queued != 2 {
			t.Fatalf("adopt of an incomplete list = %+v err %v, want X and Y queued", res, err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "y", Ref: y}}); err != nil {
			t.Fatal(err)
		}
		n := tr.add(t, "N")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("N"), []block.NamedRoot{{Name: "new", Ref: n}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "new", Previous: n}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Parked != 1 || res.Freed != 1 || res.Requeued != 0 {
			t.Fatalf("collect under unverified lists = %+v, want X parked and N freed", res)
		}
		requireStored(t, v, tr, false, "N")
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "y", Previous: y}}); err != nil {
			t.Fatal(err)
		}
		if res := collectAll(t, v); res.Parked != 0 || res.Freed != 0 {
			t.Fatalf("collect before Y's release ages = %+v, want nothing parked or freed", res)
		}
		requireStored(t, v, tr, true, "R", "P", "Z", "X", "Y")
		kind := func(k byte) []byte { return append(slices.Clone(livenessPrefix), k) }
		if j, x := countPrefix(t, store, kind('J')), countPrefix(t, store, kind('X')); j == 0 || x != 1 {
			t.Fatalf("%d journal records and %d parked, want Y's release journaled and X parked", j, x)
		}

		// A rebuild on the adopted Volume that decodes X and a P that lost
		// its child refuses P, keeps P's stored list, and leaves R and P
		// unverified, writing nothing that calls for a recount.
		rebuild := func(entries ...*block.PutBatchEntry) block_gc.ListRebuildResult {
			t.Helper()
			rb, err := v.OpenListRebuild(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if err := rb.Verify(ctx, entry.Ref, entry.Refs); err != nil {
					t.Fatal(err)
				}
			}
			res, err := rb.Finish(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return res
		}
		res := rebuild(tr["X"], &block.PutBatchEntry{Ref: p})
		if res.Dropped != 1 || !res.DroppedSample[0].EqualsRef(p) || res.Verified != 1 || res.Unverified != 1 || !res.UnverifiedSample[0].EqualsRef(r) || res.Cleared || res.Unadopted {
			t.Fatalf("rebuild with P dropping Z = %+v, want P refused and R unverified", res)
		}
		if got := livenessRecords(t, store, 'c')[string(kind('c'))+string(block_gc.BlockKey(p))]; got != string(block_gc.EncodeChildren(tr["P"].Refs)) {
			t.Fatal("refused rebuild changed P's stored list")
		}
		if n := countPrefix(t, store, kind('Y')); n != 0 {
			t.Fatalf("rebuild that rewrote nothing left %d recount records", n)
		}

		// A rerun that decodes R, P and the leaves rewrites R's list, which records the
		// recount the counts now owe, and skips X, but its walk from the roots
		// is interrupted after its first pass marked what it reached. The
		// Volume stays adopted with the mark standing, so nothing frees.
		probeCtx, cancel := context.WithCancel(ctx)
		probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){rebuildSecondPass: cancel}}
		rb, err := block_gc.OpenListRebuild(probeCtx, probe)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"X", "R", "P", "Z", "Y"} {
			if err := rb.Verify(probeCtx, tr[name].Ref, tr[name].Refs); err != nil {
				t.Fatal(err)
			}
		}
		if res, err := rb.Finish(probeCtx); !errors.Is(err, context.Canceled) || res.Verified != 4 || res.Rewritten != 1 || res.Unadopted {
			t.Fatalf("interrupted rebuild = %+v err %v, want R rewritten and the walk canceled", res, err)
		}
		if n := countPrefix(t, store, kind('Y')); n != 1 {
			t.Fatalf("rewrite on an adopted Volume left %d recount records, want 1", n)
		}
		if countPrefix(t, store, kind('K')) == 0 {
			t.Fatal("interrupted walk left no reached marks")
		}
		if res := collectAll(t, v); res.Freed != 0 {
			t.Fatalf("collect with a recount due = %+v, want nothing freed", res)
		}

		// Running it again deletes the interrupted walk's marks, verifies
		// nothing more, clears the mark, and returns the Volume to unadopted
		// in the same transaction, which frees nothing.
		if res := rebuild(); res.Verified != 0 || res.Unverified != 0 || !res.Cleared || !res.Unadopted {
			t.Fatalf("rerun rebuild = %+v, want the mark cleared and the Volume unadopted", res)
		}
		if n := countPrefix(t, store, kind('K')); n != 0 {
			t.Fatalf("finished walk left %d reached marks", n)
		}
		if got := livenessAdoption(t, v); got != block_gc.Adoption_ADOPTION_UNADOPTED {
			t.Fatalf("adoption after the rebuild = %v, want unadopted", got)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 0 || res.Requeued != 0 {
			t.Fatalf("collect while unadopted = %+v, want nothing freed", res)
		}
		requireStored(t, v, tr, true, "R", "P", "Z", "X", "Y")

		// The recount counts R's holders of X and Y and deletes every journal,
		// queued, parked, cascade, release page and recount record from before.
		if res, err := v.Adopt(ctx); err != nil || res.Queued != 0 {
			t.Fatalf("adopt after the rebuild = %+v err %v, want none queued", res, err)
		}
		for _, k := range []byte{'J', 'Q', 'X', 'C', 'D', 'Y'} {
			if n := countPrefix(t, store, kind(k)); n != 0 {
				t.Fatalf("recount left %d records of kind %c", n, k)
			}
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 0 {
			t.Fatalf("collect after the recount = %+v, want nothing freed", res)
		}
		requireStored(t, v, tr, true, "R", "P", "Z", "X", "Y")
		requireAudit(t, v, 0)

		// A rebuild of a Volume whose lists are verified is refused, since a
		// rewrite then would leave the counts short with no Finish to come.
		if _, err := v.OpenListRebuild(ctx); !errors.Is(err, block_gc.ErrListsVerified) {
			t.Fatalf("rebuild of verified lists err %v, want ErrListsVerified", err)
		}
	})

	t.Run("rewrite between the rebuild's walk and its clear", func(t *testing.T) {
		// A format 6 store holds R -> X under main with whole lists, and an
		// unreached block U whose list lacks X. Finish's walk finds every
		// reached list verified and names U suspect; before its clear,
		// another rebuild's flush
		// rewrites U's list on the adopted Volume. The clear reads that
		// flush's recount record, so it returns the Volume to unadopted.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := store_kvtx_inmem.NewStore()
		tr := testTree{}
		x := tr.add(t, "X")
		tr.add(t, "R", "X")
		u := tr.add(t, "U", "X")
		writeLegacyStore(t, store, keys, tr, "R", true, "X", "R", "U")
		kind := func(k byte, parts ...[]byte) []byte {
			return slices.Concat(append(slices.Clone(livenessPrefix), k), slices.Concat(parts...))
		}
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, kind('c', block_gc.BlockKey(u)), block_gc.EncodeChildren(nil)); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v, _ := openUnadoptedVolume(t, store)
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}

		// Once Finish has named U suspect, the next transaction is the clear:
		// before it, another rebuild reports U until a full batch flushes,
		// which rewrites U's list and must leave the recount record.
		var flushed bool
		probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
			if flushed || countPrefix(t, store, kind('Z')) == 0 {
				return
			}
			flushed = true
			other, err := v.OpenListRebuild(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; countPrefix(t, store, kind('Y')) == 0; i++ {
				if i == 1<<16 {
					t.Fatal("the other rebuild's flushes left no recount record")
				}
				if err := other.Verify(ctx, u, []*block.BlockRef{x}); err != nil {
					t.Fatal(err)
				}
			}
		}}
		rb, err := block_gc.OpenListRebuild(ctx, probe)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"X", "R"} {
			if err := rb.Verify(ctx, tr[name].Ref, tr[name].Refs); err != nil {
				t.Fatal(err)
			}
		}
		res, err := rb.Finish(ctx)
		if err != nil || !flushed || res.Rewritten != 0 || !res.Cleared || !res.Unadopted {
			t.Fatalf("rebuild = %+v err %v flushed %v, want the mark cleared and the Volume unadopted", res, err, flushed)
		}
		if got := livenessAdoption(t, v); got != block_gc.Adoption_ADOPTION_UNADOPTED {
			t.Fatalf("adoption after the rebuild = %v, want unadopted", got)
		}
	})

	t.Run("unverified list a held root reaches", func(t *testing.T) {
		// A format 6 store holds the root A and an unreached leaf Y. A root
		// moved onto Y while the lists are unverified keeps the rebuild from
		// clearing, naming Y and the root, until the root is released; the
		// next Finish then clears and names Y suspect.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := store_kvtx_inmem.NewStore()
		tr := testTree{}
		tr.add(t, "A")
		y := tr.add(t, "Y")
		writeLegacyStore(t, store, keys, tr, "A", true, "A", "Y")
		v, _ := openUnadoptedVolume(t, store)
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "y", Ref: y}}); err != nil {
			t.Fatal(err)
		}
		rebuild := func() block_gc.ListRebuildResult {
			t.Helper()
			rb, err := v.OpenListRebuild(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := rb.Verify(ctx, tr["A"].Ref, tr["A"].Refs); err != nil {
				t.Fatal(err)
			}
			res, err := rb.Finish(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return res
		}
		want := block_gc.UnverifiedRoot{BucketID: "b", Name: "y", Unverified: 1}
		if res := rebuild(); res.Cleared || res.Unverified != 1 || !res.UnverifiedSample[0].EqualsRef(y) || len(res.UnverifiedRoots) != 1 || res.UnverifiedRoots[0] != want {
			t.Fatalf("rebuild with y held = %+v, want Y unverified from b/y", res)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "y", Previous: y}}); err != nil {
			t.Fatal(err)
		}
		if res := rebuild(); !res.Cleared || res.Suspect != 1 {
			t.Fatalf("rebuild with y released = %+v, want the mark cleared with Y suspect", res)
		}
	})

	t.Run("hold between the rebuild's last pass and its clear", func(t *testing.T) {
		// A format 6 store holds the root A and an unreached leaf U. Once
		// the walk's first pass has marked A and the second has begun, a
		// root moves onto U before the second pass drains, after it read
		// the roots. The pass reaches
		// nothing new and markSuspects names U suspect, but the clear finds
		// the old holds moved, so Finish passes again, reaches U from the
		// new root and refuses to clear with U unverified.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := store_kvtx_inmem.NewStore()
		tr := testTree{}
		tr.add(t, "A")
		u := tr.add(t, "U")
		writeLegacyStore(t, store, keys, tr, "A", true, "A", "U")
		v, _ := openUnadoptedVolume(t, store)
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		reached := slices.Concat(livenessPrefix, []byte{'K'})
		var held bool
		probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
			if held || countPrefix(t, store, reached) == 0 || livenessListPass(t, v) < 2 {
				return
			}
			held = true
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "u", Ref: u}}); err != nil {
				t.Fatal(err)
			}
		}}
		rb, err := block_gc.OpenListRebuild(ctx, probe)
		if err != nil {
			t.Fatal(err)
		}
		if err := rb.Verify(ctx, tr["A"].Ref, tr["A"].Refs); err != nil {
			t.Fatal(err)
		}
		res, err := rb.Finish(ctx)
		want := block_gc.UnverifiedRoot{BucketID: "b", Name: "u", Unverified: 1}
		if err != nil || !held || res.Cleared || res.Passes != 1 || res.Unverified != 1 || !res.UnverifiedSample[0].EqualsRef(u) || len(res.UnverifiedRoots) != 1 || res.UnverifiedRoots[0] != want {
			t.Fatalf("rebuild = %+v err %v held %v, want one more pass and U unverified from b/u", res, err, held)
		}
	})

	t.Run("hold onto a block born before the rebuild's last pass", func(t *testing.T) {
		// A format 6 store holds the root A and an unreached leaf U. While
		// the mark stands, a Land gives birth to X naming U under the root x,
		// and x is released, so X is stored with a landed mark and nothing
		// holds it. Once Finish's second pass has read the roots, a root
		// moves onto X. X was born before that pass, so the hold counts as
		// old: the clear refuses, and Finish passes again and reaches U
		// through X.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := store_kvtx_inmem.NewStore()
		tr := testTree{}
		tr.add(t, "A")
		u := tr.add(t, "U")
		writeLegacyStore(t, store, keys, tr, "A", true, "A", "U")
		v, _ := openUnadoptedVolume(t, store)
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		x := tr.add(t, "X", "U")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X"), []block.NamedRoot{{Name: "x", Ref: x}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "x", Previous: x}}); err != nil {
			t.Fatal(err)
		}
		reached := slices.Concat(livenessPrefix, []byte{'K'})
		var held bool
		probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
			if held || countPrefix(t, store, reached) == 0 || livenessListPass(t, v) < 2 {
				return
			}
			held = true
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "x2", Ref: x}}); err != nil {
				t.Fatal(err)
			}
		}}
		rb, err := block_gc.OpenListRebuild(ctx, probe)
		if err != nil {
			t.Fatal(err)
		}
		if err := rb.Verify(ctx, tr["A"].Ref, tr["A"].Refs); err != nil {
			t.Fatal(err)
		}
		res, err := rb.Finish(ctx)
		want := block_gc.UnverifiedRoot{BucketID: "b", Name: "x2", Unverified: 1}
		if err != nil || !held || res.Cleared || res.Passes != 1 || res.Unverified != 1 || !res.UnverifiedSample[0].EqualsRef(u) || len(res.UnverifiedRoots) != 1 || res.UnverifiedRoots[0] != want {
			t.Fatalf("rebuild = %+v err %v held %v, want one more pass and U unverified from b/x2", res, err, held)
		}
	})

	t.Run("put between the suspect marking's slices", func(t *testing.T) {
		// A format 6 store holds the root A and more unreached leaves than
		// one slice of markSuspects lists. Between its first and second
		// slice, a put stores again a leaf the join has passed, which has a
		// suspect record, and one ahead of it, which has none. Both end
		// clean or pending and visible, a leaf nothing put stays hidden, and
		// the suspect count matches the records.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := store_kvtx_inmem.NewStore()
		tr := testTree{}
		tr.add(t, "A")
		names := []string{"A"}
		for i := range 300 {
			name := "L" + strconv.Itoa(i)
			tr.add(t, name)
			names = append(names, name)
		}
		writeLegacyStore(t, store, keys, tr, "A", true, names...)
		v, _ := openUnadoptedVolume(t, store)
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		suspects := slices.Concat(livenessPrefix, []byte{'Z'})
		hasRecord := func(name string) bool {
			tx, err := store.NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			found, err := tx.Exists(ctx, slices.Concat(suspects, block_gc.BlockKey(tr[name].Ref)))
			if err != nil {
				t.Fatal(err)
			}
			return found
		}
		var behind, ahead string
		probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
			if behind != "" || countPrefix(t, store, suspects) == 0 {
				return
			}
			for _, name := range names[1:] {
				switch {
				case behind == "" && hasRecord(name):
					behind = name
				case ahead == "" && !hasRecord(name):
					ahead = name
				}
			}
			if behind == "" || ahead == "" {
				t.Fatalf("after the first slice behind %q ahead %q, want one of each", behind, ahead)
			}
			if _, err := v.PutBlockBatch(ctx, tr.entries(behind, ahead)); err != nil {
				t.Fatal(err)
			}
		}}
		rb, err := block_gc.OpenListRebuild(ctx, probe)
		if err != nil {
			t.Fatal(err)
		}
		if err := rb.Verify(ctx, tr["A"].Ref, tr["A"].Refs); err != nil {
			t.Fatal(err)
		}
		res, err := rb.Finish(ctx)
		if err != nil || behind == "" || !res.Cleared {
			t.Fatalf("rebuild = %+v err %v put %q %q, want the mark cleared after the put", res, err, behind, ahead)
		}
		if n := countPrefix(t, store, suspects); uint64(n) != res.Suspect {
			t.Fatalf("%d suspect records, state counts %d", n, res.Suspect)
		}
		untouched := names[1]
		if untouched == behind || untouched == ahead {
			untouched = names[len(names)-1]
		}
		for name, want := range map[string]bool{behind: true, ahead: true, untouched: false} {
			if found, err := v.GetBlockExists(ctx, tr[name].Ref); err != nil || found != want {
				t.Fatalf("exists %s = %v err %v, want %v", name, found, err, want)
			}
		}
	})

	t.Run("reput of an older block stored without a list", func(t *testing.T) {
		// A format 6 store holds Q under main, naming P, but a crash left Q's
		// list without P and P stored without a list of its own, so the
		// adoption passes and counts nothing for P. A put of P again under
		// the new N writes P's list, and Land keeps one holder more for P,
		// since its bytes were stored. P's bytes predate the put, so P is
		// older and gets no landed mark: once a later adoption recounts the
		// lists and finds P at zero, the free parks it, and Q still reaches
		// it.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := store_kvtx_inmem.NewStore()
		tr := testTree{}
		tr.add(t, "P")
		q := tr.add(t, "Q", "P")
		writeLegacyStore(t, store, keys, tr, "Q", true, "Q")
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, append([]byte{livenessPrefix[0], 'c'}, block_gc.BlockKey(q)...), block_gc.EncodeChildren(nil)); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v, clk := openUnadoptedVolume(t, store)
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}

		// Put P again under N, then release N: the free takes N and keeps P
		// at the holder the reput kept.
		n := tr.add(t, "N", "P")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P", "N"), []block.NamedRoot{{Name: "new", Ref: n}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "new", Previous: n}}); err != nil {
			t.Fatal(err)
		}
		if _, landed := livenessRecords(t, store, 'W')[string(append([]byte{livenessPrefix[0], 'W'}, block_gc.BlockKey(tr["P"].Ref)...))]; landed {
			t.Fatal("the reput marked P landed")
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 1 || res.Parked != 0 {
			t.Fatalf("collect after the reput = %+v, want N freed and P kept", res)
		}
		requireStored(t, v, tr, true, "Q", "P")
		requireStored(t, v, tr, false, "N")

		// A recount finds P at zero, and the free parks it.
		markUnadopted(t, v)
		if res, err := v.Adopt(ctx); err != nil || res.Queued != 1 {
			t.Fatalf("readopt = %+v err %v, want P queued", res, err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Parked != 1 || res.Freed != 0 {
			t.Fatalf("collect after the recount = %+v, want P parked", res)
		}
		requireStored(t, v, tr, true, "Q", "P")
	})

	t.Run("parked block requeued once the lists verify", func(t *testing.T) {
		// A format 6 store holds R -> X under main and an unreached old block
		// G. Adopted with its lists unverified, the free parks G, an older
		// block. Once the mark clears, the next pass moves G back onto the
		// queue and frees it, and keeps R and X.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := store_kvtx_inmem.NewStore()
		tr := testTree{}
		tr.add(t, "X")
		tr.add(t, "R", "X")
		tr.add(t, "G")
		writeLegacyStore(t, store, keys, tr, "R", true, "X", "R", "G")
		v, clk := openUnadoptedVolume(t, store)
		if res, err := v.Adopt(ctx); err != nil || res.Queued != 1 {
			t.Fatalf("adopt = %+v err %v, want G queued", res, err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Parked != 1 || res.Freed != 0 {
			t.Fatalf("collect under unverified lists = %+v, want G parked", res)
		}
		requireStored(t, v, tr, true, "R", "X", "G")
		setListsUnverified(t, v, false)
		if res := collectAll(t, v); res.Requeued != 1 || res.Freed != 1 || res.Parked != 0 {
			t.Fatalf("collect once verified = %+v, want G requeued and freed", res)
		}
		requireStored(t, v, tr, true, "R", "X")
		requireStored(t, v, tr, false, "G")
		requireAudit(t, v, 0)
	})

	t.Run("parked block a commit names before the clear", func(t *testing.T) {
		// A format 6 store holds the root A and an unreached block C -> F
		// whose list lacks F. The adoption counts no holder of C or F, and
		// the free parks both. A commit then names F from a new block C2,
		// journaling F's up pair, and the rebuild clears the mark with C's
		// list still cut, since no root reaches C. The clear's hold mark
		// keeps F until that up pair nets; only C is freed.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := store_kvtx_inmem.NewStore()
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "F")
		c := tr.add(t, "C", "F")
		writeLegacyStore(t, store, keys, tr, "A", true, "A", "F", "C")
		kind := func(k byte, parts ...[]byte) []byte {
			return slices.Concat(append(slices.Clone(livenessPrefix), k), slices.Concat(parts...))
		}
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, kind('c', block_gc.BlockKey(c)), block_gc.EncodeChildren(nil)); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v, clk := openUnadoptedVolume(t, store)
		if res, err := v.Adopt(ctx); err != nil || res.Queued != 2 {
			t.Fatalf("adopt = %+v err %v, want C and F queued", res, err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Parked != 2 || res.Freed != 0 {
			t.Fatalf("collect under unverified lists = %+v, want C and F parked", res)
		}

		// C2 -> F lands under a new root while F is parked at zero.
		c2 := tr.add(t, "C2", "F")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C2"), []block.NamedRoot{{Name: "c2", Ref: c2}}); err != nil {
			t.Fatal(err)
		}
		rb, err := v.OpenListRebuild(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"A", "F"} {
			if err := rb.Verify(ctx, tr[name].Ref, tr[name].Refs); err != nil {
				t.Fatal(err)
			}
		}
		if res, err := rb.Finish(ctx); err != nil || !res.Cleared || res.Unadopted {
			t.Fatalf("rebuild = %+v err %v, want the mark cleared on the adopted Volume", res, err)
		}

		// The requeued blocks wait for the up pair; once it nets, F has its
		// holder and only C is freed.
		if res := collectAll(t, v); res.Requeued != 2 || res.Freed != 0 {
			t.Fatalf("collect after the clear = %+v, want C and F requeued and kept", res)
		}
		requireStored(t, v, tr, true, "A", "C", "C2", "F")
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 1 {
			t.Fatalf("collect once the up pair ages = %+v, want only C freed", res)
		}
		requireStored(t, v, tr, true, "A", "C2", "F")
		requireStored(t, v, tr, false, "C")
		requireAudit(t, v, 0)
	})

	t.Run("suspect list a put revives", func(t *testing.T) {
		// A format 6 store holds the root A and unreached blocks C -> F,
		// whose list lacks F, G and H. The rebuild clears the mark with C's
		// and G's lists unverified, so Finish names them and H suspect, and F,
		// verified but unreached, pending. A second Finish is refused. A put of G,
		// whose list is whole, leaves the Volume adopted. A put of C under a
		// root rewrites C's list to name F and returns the Volume to
		// unadopted, asking for a recount; once it runs, F keeps its holder,
		// H, which no put revived, still reads missing, and the free of H
		// drops its suspect record.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := store_kvtx_inmem.NewStore()
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "F")
		tr.add(t, "G")
		tr.add(t, "H")
		c := tr.add(t, "C", "F")
		writeLegacyStore(t, store, keys, tr, "A", true, "A", "F", "G", "H", "C")
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		cut := slices.Concat(livenessPrefix, []byte{'c'}, block_gc.BlockKey(c))
		if err := tx.Set(ctx, cut, block_gc.EncodeChildren(nil)); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v, clk := openUnadoptedVolume(t, store)
		v.adoptWake = make(chan struct{}, 1)
		if res, err := v.Adopt(ctx); err != nil || res.Queued != 4 {
			t.Fatalf("adopt = %+v err %v, want C, F, G and H queued", res, err)
		}
		rb, err := v.OpenListRebuild(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"A", "F"} {
			if err := rb.Verify(ctx, tr[name].Ref, tr[name].Refs); err != nil {
				t.Fatal(err)
			}
		}
		if res, err := rb.Finish(ctx); err != nil || !res.Cleared || res.Unadopted || res.Suspect != 4 {
			t.Fatalf("rebuild = %+v err %v, want the mark cleared with C, G and H suspect and F pending", res, err)
		}
		suspects := slices.Concat(livenessPrefix, []byte{'Z'})
		if _, err := rb.Finish(ctx); !errors.Is(err, block_gc.ErrListsVerified) {
			t.Fatalf("second finish err %v, want ErrListsVerified", err)
		}
		if n := countPrefix(t, store, suspects); n != 4 {
			t.Fatalf("suspect records after the second finish = %d, want 4", n)
		}
		woke := func() bool {
			select {
			case <-v.adoptWake:
				return true
			default:
				return false
			}
		}

		// The existence check hides the suspect C, not the pending F, and a
		// root onto C without its bytes is refused.
		for name, want := range map[string]bool{"A": true, "C": false, "F": true} {
			if found, err := v.GetBlockExists(ctx, tr[name].Ref); err != nil || found != want {
				t.Fatalf("exists %s = %v err %v, want %v", name, found, err, want)
			}
		}
		var suspect *block_gc.SuspectError
		err = v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "c", Ref: c}})
		if !errors.As(err, &suspect) {
			t.Fatalf("root onto the suspect C err %v, want a SuspectError", err)
		}

		// G's list is whole: the put trusts it once checked.
		g := tr["G"].Ref
		if err := v.SetBucketRoots(ctx, "b", tr.entries("G"), []block.NamedRoot{{Name: "g", Ref: g}}); err != nil {
			t.Fatal(err)
		}
		if a := livenessAdoption(t, v); a != block_gc.Adoption_ADOPTION_ADOPTED || woke() {
			t.Fatalf("after the put of G the Volume is %v, want adopted and no recount", a)
		}

		// C's list lacks F: the put rewrites it and asks for a recount.
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "c", Ref: c}}); err != nil {
			t.Fatal(err)
		}
		if a := livenessAdoption(t, v); a != block_gc.Adoption_ADOPTION_UNADOPTED || !woke() {
			t.Fatalf("after the put of C the Volume is %v, want unadopted and a recount asked for", a)
		}
		if found, err := v.GetBlockExists(ctx, c); err != nil || !found {
			t.Fatalf("exists C after its put = %v err %v, want found", found, err)
		}
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		if found, err := v.GetBlockExists(ctx, tr["H"].Ref); err != nil || found {
			t.Fatalf("exists H after the recount = %v err %v, want still suspect", found, err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 1 {
			t.Fatalf("collect after the recount = %+v, want H freed", res)
		}
		requireStored(t, v, tr, true, "A", "C", "F", "G")
		requireStored(t, v, tr, false, "H")
		if n := countPrefix(t, store, suspects); n != 0 {
			t.Fatalf("suspect records after the free = %d, want none", n)
		}
		requireAudit(t, v, 0)
	})

	t.Run("suspect put supersedes an overlapping adoption", func(t *testing.T) {
		// The store of "suspect list a put revives", unadopted after the
		// format 7 upgrade, with a rebuild that verified A and F. Adoption A
		// marks the Volume adopting, then B claims it and counts the lists,
		// C's still lacking F. Before B settles, a put of C rewrites C's list
		// and returns the Volume to unadopted. Neither run may settle: B
		// counted the short list and B's claim superseded A. After the
		// recount the put asks for, Finish leaves only G suspect, since the
		// put marked C's list; the free takes G, which nothing names, and F keeps
		// its holder.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := store_kvtx_inmem.NewStore()
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "F")
		tr.add(t, "G")
		c := tr.add(t, "C", "F")
		writeLegacyStore(t, store, keys, tr, "A", true, "A", "F", "G", "C")
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		cut := slices.Concat(livenessPrefix, []byte{'c'}, block_gc.BlockKey(c))
		if err := tx.Set(ctx, cut, block_gc.EncodeChildren(nil)); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v, clk := openUnadoptedVolume(t, store)
		v.adoptWake = make(chan struct{}, 1)
		rb, err := v.OpenListRebuild(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"A", "F"} {
			if err := rb.Verify(ctx, tr[name].Ref, tr[name].Refs); err != nil {
				t.Fatal(err)
			}
		}

		// B runs inside A, after A's mark; the put of C lands inside B,
		// after B counted the lists.
		var errB error
		probeB := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptSettle: func() {
				if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "c", Ref: c}}); err != nil {
					t.Error(err)
				}
			},
		}}
		probeA := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptCountRoots: func() { _, errB = block_gc.Adopt(ctx, probeB) },
		}}
		_, errA := block_gc.Adopt(ctx, probeA)
		if !errors.Is(errA, block_gc.ErrAdoptSuperseded) || !errors.Is(errB, block_gc.ErrAdoptSuperseded) {
			t.Fatalf("adoptions = %v and %v with %d and %d hooks left, want both superseded", errA, errB, len(probeA.at), len(probeB.at))
		}
		if a := livenessAdoption(t, v); a != block_gc.Adoption_ADOPTION_UNADOPTED {
			t.Fatalf("after the put of C the Volume is %v, want unadopted", a)
		}
		select {
		case <-v.adoptWake:
		default:
			t.Fatal("the put of C asked for no recount")
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 0 {
			t.Fatalf("collect before the recount = %+v, want nothing freed", res)
		}
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		if res, err := rb.Finish(ctx); err != nil || !res.Cleared || res.Suspect != 1 {
			t.Fatalf("rebuild = %+v err %v, want the mark cleared with G suspect", res, err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 1 {
			t.Fatalf("collect after the recount = %+v, want G freed", res)
		}
		requireStored(t, v, tr, true, "A", "C", "F")
		requireStored(t, v, tr, false, "G")
		requireAudit(t, v, 0)
	})

	t.Run("adoption claimed by another", func(t *testing.T) {
		// main -> R -> X on an unadopted Volume. Adoption A clears, then B,
		// another process, starts while the Volume is still unadopted. A
		// marks the Volume adopting and counts the roots; B clears A's
		// counts and dies before it marks. A must not settle the cleared
		// counts: B's claim superseded it, and nothing main reaches is freed.
		store := store_kvtx_inmem.NewStore()
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}

		// B claims, waits before its first clear, clears on release, and dies
		// before it marks.
		claimed, cleared, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
		releaseB := sync.OnceFunc(func() { close(release) })
		defer releaseB()
		bCtx, bCancel := context.WithCancel(ctx)
		defer bCancel()
		probeB := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptClearCounts: func() { close(claimed); <-release },
			adoptMark:        bCancel,
		}}
		go func() {
			defer close(cleared)
			_, _ = block_gc.Adopt(bCtx, probeB)
		}()

		// A starts B before its mark and lets B clear before it settles.
		probeA := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptMark: func() { <-claimed },
			adoptSettle: func() {
				releaseB()
				<-cleared
			},
		}}
		_, err := block_gc.Adopt(ctx, probeA)
		releaseB()
		<-cleared
		if !errors.Is(err, block_gc.ErrAdoptSuperseded) {
			t.Fatalf("claimed adoption = %v, want superseded", err)
		}

		// Nothing is freed, and a clean adoption keeps main's tree.
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")
		requireAudit(t, v, 0)
	})

	t.Run("overlapping adoptions", func(t *testing.T) {
		// main -> R -> X on an unadopted Volume. Adoption A counts the
		// holders, and before it settles them a second adoption B runs to
		// the end. A must not settle B's counts again: it refuses, B's
		// adoption stands, and nothing main reaches is freed.
		store := store_kvtx_inmem.NewStore()
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptSettle: func() {
				if _, err := block_gc.Adopt(ctx, collectorStore{v: v}); err != nil {
					t.Error(err)
				}
			},
		}}
		if _, err := block_gc.Adopt(ctx, probe); !errors.Is(err, block_gc.ErrAdoptSuperseded) || len(probe.at) != 0 {
			t.Fatalf("superseded adopt = %v with %d adoptions left, want superseded", err, len(probe.at))
		}
		if adoption := livenessAdoption(t, v); adoption != block_gc.Adoption_ADOPTION_ADOPTED {
			t.Fatalf("overlapping adoptions left %v, want adopted", adoption)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")
		requireAudit(t, v, 0)
	})
}

// rebuildSecondPass is the transaction of a list rebuild's Finish, on a
// Volume whose records fit in one slice, that starts the walk's second pass:
// after the flush, the clear of earlier marks, and the first pass's start and
// drain.
const rebuildSecondPass = 5

// The transactions of block_gc.Adopt, in order, on a Volume whose records
// each fit in one slice: the claim, the clears of counts, the queue, parked
// blocks, cascade records, born marks, moved-root marks, old fill marks, release pages, fill
// ups, the old up index, recount records and fill records, then the mark, the root count, the
// child list count and the settle.
const (
	// adoptClearCounts clears the count records an earlier run left.
	adoptClearCounts = 2
	// adoptMark marks the Volume adopting.
	adoptMark = 13
	// adoptCountRoots counts the roots.
	adoptCountRoots = 14
	// adoptCountChildren counts the child lists' holders.
	adoptCountChildren = 15
	// adoptSettle settles the stored blocks' counts.
	adoptSettle = 16
)

// auditProbe runs before, once, when an audit scans the stored bytes, which
// it does after its snapshot and before it records the holes.
type auditProbe struct {
	collectorStore
	// before runs before the scan, nil once it ran.
	before func()
}

// ScanBlocks runs before, then scans the stored bytes.
func (p *auditProbe) ScanBlocks(ctx context.Context, fn func(refs []*block.BlockRef) error) error {
	if before := p.before; before != nil {
		p.before = nil
		before()
	}
	return p.collectorStore.ScanBlocks(ctx, fn)
}

// adoptProbe runs an adoption's transactions, calling the func at a
// transaction's position, counted from one, before it runs.
type adoptProbe struct {
	block_gc.Store
	// at maps a transaction's position to the func called before it.
	at map[int]func()
	// n counts the transactions.
	n int
}

// Update calls the func at the transaction's position, then runs it.
func (p *adoptProbe) Update(ctx context.Context, fn func(rec *block_gc.Records, blocks block.StoreOps) error) error {
	p.n++
	if f := p.at[p.n]; f != nil {
		delete(p.at, p.n)
		f()
	}
	return p.Store.Update(ctx, fn)
}

// waitAdopted waits for the background adoption of v to mark it adopted.
func waitAdopted(t *testing.T, v *Volume) {
	t.Helper()
	if v.adoptDone == nil {
		t.Fatal("the Volume runs no background adoption")
	}
	for deadline := time.Now().Add(10 * time.Second); livenessAdoption(t, v) != block_gc.Adoption_ADOPTION_ADOPTED; {
		if time.Now().After(deadline) {
			t.Fatalf("background adoption left the Volume %v", livenessAdoption(t, v))
		}
		time.Sleep(time.Millisecond)
	}
}

// livenessListPass returns the walk passes of list rebuilds the state of v
// counts.
func livenessListPass(t *testing.T, v *Volume) uint64 {
	t.Helper()
	var pass uint64
	err := v.readRecords(t.Context(), func(rec *block_gc.Records) error {
		state, err := rec.GetState(t.Context())
		pass = state.GetListPass()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return pass
}

// livenessAdoption returns the Volume's adoption state.
func livenessAdoption(t *testing.T, v *Volume) block_gc.Adoption {
	t.Helper()
	var adoption block_gc.Adoption
	err := v.readRecords(t.Context(), func(rec *block_gc.Records) error {
		state, err := rec.GetState(t.Context())
		adoption = state.GetAdoption()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return adoption
}

// setListsUnverified sets the Volume's unverified child lists mark, as an
// upgrade from format 6 does, or clears it, as a repair that rebuilt the lists
// from the blocks does.
func setListsUnverified(t *testing.T, v *Volume, unverified bool) {
	t.Helper()
	err := collectorStore{v: v}.Update(t.Context(), func(rec *block_gc.Records, _ block.StoreOps) error {
		state, err := rec.GetState(t.Context())
		if err != nil {
			return err
		}
		state.UnverifiedLists = unverified
		return rec.SetState(t.Context(), state)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// markUnadopted returns an adopted Volume to unadopted, as one that predates
// adoption, keeping its records.
func markUnadopted(t *testing.T, v *Volume) {
	t.Helper()
	err := collectorStore{v: v}.Update(t.Context(), func(rec *block_gc.Records, _ block.StoreOps) error {
		state, err := rec.GetState(t.Context())
		if err != nil {
			return err
		}
		state.Adoption = block_gc.Adoption_ADOPTION_UNADOPTED
		return rec.SetState(t.Context(), state)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// walkProbe calls before ahead of each of a store's write transactions.
type walkProbe struct {
	block_gc.Store
	// before runs ahead of each transaction.
	before func()
}

// MarkingSuspects opens the probed store's suspect gate, as Finish asks.
func (p *walkProbe) MarkingSuspects(ctx context.Context) (func(), error) {
	return p.Store.(block_gc.SuspectMarker).MarkingSuspects(ctx)
}

// Update calls before, then runs the transaction.
func (p *walkProbe) Update(ctx context.Context, fn func(rec *block_gc.Records, blocks block.StoreOps) error) error {
	p.before()
	return p.Store.Update(ctx, fn)
}

// livenessRecords returns the liveness records of the kinds in store, by key.
func livenessRecords(t *testing.T, store *store_kvtx_inmem.Store, kinds ...byte) map[string]string {
	t.Helper()
	tx, err := store.NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	out := make(map[string]string)
	for _, kind := range kinds {
		err := tx.ScanPrefix(t.Context(), append(slices.Clone(livenessPrefix), kind), func(key, value []byte) error {
			out[string(key)] = string(value)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// forgetPins drops this process's record of its reader pins, as a process
// that died leaves them: nothing renews or releases their roots.
func forgetPins(t *testing.T, v *Volume) {
	t.Helper()
	unlock, err := v.rootPinMu.Lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	clear(v.rootPins)
	clear(v.idlePins)
	unlock()
}

// advanceRenewing moves the clock forward by d in steps shorter than the
// lease, renewing this process's leased roots after each, as its renewal
// loop does.
func advanceRenewing(t *testing.T, v *Volume, clk *testClock, d time.Duration) {
	t.Helper()
	for d > 0 {
		step := min(d, rootRenewInterval)
		clk.Add(step)
		d -= step
		if _, err := v.renewRoots(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
}

// setUnadopted marks store, which must be empty, as a Volume of the current
// format that predates adoption.
func setUnadopted(t *testing.T, store *store_kvtx_inmem.Store) {
	t.Helper()
	keys := store_kvkey.NewDefaultKVKey()
	state := &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_UNADOPTED}
	if err := writeFormat(t.Context(), store, keys, state); err != nil {
		t.Fatal(err)
	}
}

// TestFormatUpgradeCarriesLiveness opens a store of format 6 and checks that
// format 7 keeps its child lists in place, carries its named roots into the
// root table, leaves it unadopted, sweeps the rest of the old graph, and that
// an adoption then counts its holders. A second store whose root names a block
// stored without a child list refuses adoption.
func TestFormatUpgradeCarriesLiveness(t *testing.T) {
	// Write a format 6 store: R1 -> C, its child lists, a root owner edge
	// holding R1, and old graph records.
	ctx := t.Context()
	keys := store_kvkey.NewDefaultKVKey()
	store := store_kvtx_inmem.NewStore()
	tr := testTree{}
	tr.add(t, "C")
	r1 := tr.add(t, "R1", "C")
	writeLegacyStore(t, store, keys, tr, "R1", true, "C", "R1")

	// Open it: the lists and the root carry over, and it is unadopted.
	v, _ := openUnadoptedVolume(t, store)
	if v.legacyDone != nil {
		<-v.legacyDone
	}
	if got, err := v.GetBucketRoot(ctx, "space", "world"); err != nil || !got.EqualsRef(r1) {
		t.Fatalf("carried root = %v err %v, want R1", got, err)
	}
	if children, listed, err := v.GetChildren(ctx, r1); err != nil || !listed || len(children) != 1 || !children[0].EqualsRef(tr["C"].Ref) {
		t.Fatalf("carried child list = %v listed %v err %v", children, listed, err)
	}
	err := v.readRecords(ctx, func(rec *block_gc.Records) error {
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		if state.GetAdoption() != block_gc.Adoption_ADOPTION_UNADOPTED || state.GetLegacyGraph() || !state.GetUnverifiedLists() {
			t.Fatalf("state %v, want unadopted, unverified, with the old graph swept", state)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range legacyGraphKinds {
		if n := countPrefix(t, store, append([]byte{livenessPrefix[0]}, kind)); n != 0 {
			t.Fatalf("%d old graph records of kind %q remain", n, kind)
		}
	}

	// Adoption counts the unverified lists as they are: nothing is garbage.
	res, err := v.Adopt(ctx)
	if err != nil || res.Blocks != 2 || res.Roots != 1 || res.Queued != 0 {
		t.Fatalf("adopt = %+v err %v, want 2 blocks, 1 root, none queued", res, err)
	}
	requireAudit(t, v, 0)

	// A store whose root names a block stored without a child list refuses
	// adoption and stays unadopted.
	store2 := store_kvtx_inmem.NewStore()
	writeLegacyStore(t, store2, keys, tr, "R1", false)
	v2, _ := openUnadoptedVolume(t, store2)
	setListsUnverified(t, v2, false)
	if _, err := v2.Adopt(ctx); !errors.Is(err, block_gc.ErrAdoptUnknown) {
		t.Fatalf("adopt of a listless root = %v, want ErrAdoptUnknown", err)
	}
	if res, err := v2.Collect(ctx); err != nil || res.Netted != 0 {
		t.Fatalf("collect after a refused adoption netted %d err %v", res.Netted, err)
	}

	// A format 6 store opened with frees on adopts in the background with
	// its lists unverified, which the generation rule makes safe.
	store4 := store_kvtx_inmem.NewStore()
	writeLegacyStore(t, store4, keys, tr, "R1", true, "C", "R1")
	v4, _ := openLivenessVolume(t, store4, true)
	waitAdopted(t, v4)

	// An adopted format 7 store where main holds B, which a fill stored
	// naming C with a fill mark and no holder for C, opens unadopted, and its
	// adoption counts C under B and deletes the mark.
	store3 := store_kvtx_inmem.NewStore()
	tr3 := testTree{}
	tr3.add(t, "C")
	b := tr3.add(t, "B", "C")
	writeFormat7Fill(t, store3, keys, tr3, b)
	v3, clk3 := openUnadoptedVolume(t, store3)
	err = v3.readRecords(ctx, func(rec *block_gc.Records) error {
		state, err := rec.GetState(ctx)
		if err == nil && state.GetAdoption() != block_gc.Adoption_ADOPTION_UNADOPTED {
			t.Fatalf("format 7 state %v, want unadopted", state)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if res, err := v3.Adopt(ctx); err != nil || res.Blocks != 2 || res.Queued != 0 {
		t.Fatalf("adopt of format 7 = %+v err %v, want 2 blocks, none queued", res, err)
	}
	if n := countPrefix(t, store3, []byte{livenessPrefix[0], 'F'}); n != 0 {
		t.Fatalf("%d fill marks remain after the adoption", n)
	}
	clk3.Add(block_gc.DefaultGrace + time.Second)
	collectAll(t, v3)
	requireStored(t, v3, tr3, true, "B", "C")
	requireAudit(t, v3, 0)
}

// writeFormat7Fill writes an adopted format 7 store holding the blocks of tr
// with their child lists, a named root main of bucket b on root, and a fill
// mark on root, as a format 7 fill left a block whose children it gave no
// holder.
func writeFormat7Fill(t *testing.T, store *store_kvtx_inmem.Store, keys *store_kvkey.KVKey, tr testTree, root *block.BlockRef) {
	t.Helper()
	ctx := t.Context()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Write the blocks, their lists, the root, the mark and the state.
	rec := block_gc.NewRecords(tx, livenessPrefix)
	for _, entry := range tr {
		key, err := entry.Ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, keys.GetBlockKey(key), entry.Data); err != nil {
			t.Fatal(err)
		}
		if err := rec.PutChildren(ctx, entry.Ref, entry.Refs); err != nil {
			t.Fatal(err)
		}
	}
	if err := rec.SetRoot(ctx, "b", "main", &block_gc.RootEntry{Ref: block_gc.BlockKey(root)}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, append([]byte{livenessPrefix[0], 'F'}, block_gc.BlockKey(root)...), nil); err != nil {
		t.Fatal(err)
	}
	if err := rec.SetState(ctx, &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_ADOPTED}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(7)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// writeLegacyStore writes a format 6 store holding the blocks of tr, a named
// root world of bucket space on root, the child lists of listed when lists is
// set, and a few old graph records of other kinds.
func writeLegacyStore(t *testing.T, store *store_kvtx_inmem.Store, keys *store_kvkey.KVKey, tr testTree, root string, lists bool, listed ...string) {
	t.Helper()
	ctx := t.Context()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	set := func(key, value []byte) {
		if err := tx.Set(ctx, key, value); err != nil {
			t.Fatal(err)
		}
	}

	// Write the version, the blocks and their child lists.
	set(keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(6))
	for _, entry := range tr {
		key, err := entry.Ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		set(keys.GetBlockKey(key), entry.Data)
	}
	if lists {
		for _, name := range listed {
			key := append([]byte{livenessPrefix[0], 'c'}, block_gc.BlockKey(tr[name].Ref)...)
			set(key, block_gc.EncodeChildren(tr[name].Refs))
		}
	}

	// Write the root owner edge and old records of other kinds.
	owner := legacyRootOwnerPrefix + base64.RawURLEncoding.EncodeToString([]byte("space")) + "/" + base64.RawURLEncoding.EncodeToString([]byte("world"))
	edge := append([]byte{livenessPrefix[0], legacyForwardKind, legacyTextNode}, binary.AppendUvarint(nil, uint64(len(owner)))...)
	edge = append(append(edge, owner...), block_gc.BlockKey(tr[root].Ref)...)
	set(edge, nil)
	set(append([]byte{livenessPrefix[0], 'h'}, block_gc.BlockKey(tr[root].Ref)...), []byte{1})
	set([]byte{livenessPrefix[0], 'w'}, []byte("sharing"))
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// countPrefix counts the keys of store under prefix.
func countPrefix(t *testing.T, store *store_kvtx_inmem.Store, prefix []byte) int {
	t.Helper()
	tx, err := store.NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	var n int
	err = tx.ScanPrefixKeys(t.Context(), prefix, func([]byte) error {
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestSuspectGate checks the suspect gate's ordering, the invariant its doc
// states: a reader skips suspect reads only after a zero count no Finish
// overlaps. A close from an epoch a Finish moved past fails, a close while a
// Finish runs fails, an assumer is refused while the gate is open, and a
// Finish entering waits for the assumers that saw the gate closed.
func TestSuspectGate(t *testing.T) {
	var g suspectGate

	// The zero gate is open, and a close from its epoch closes it.
	e, closed := g.load()
	if closed {
		t.Fatal("zero gate is closed, want open")
	}
	g.close(e)
	if _, closed := g.load(); !closed {
		t.Fatal("close from the open epoch left the gate open")
	}

	// An assumer holds back a Finish until it ends.
	ok, release := g.assume()
	if !ok {
		t.Fatal("assume on a closed gate refused")
	}
	// A Finish whose context ends while it waits returns, the gate open.
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := g.marking(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("marking with an ended context err %v, want context.Canceled", err)
	}
	if _, closed := g.load(); closed || g.running.Load() != 0 {
		t.Fatalf("after a cancelled marking the gate is closed %v with %d running, want open with none", closed, g.running.Load())
	}

	entered := make(chan func())
	go func() {
		end, err := g.marking(t.Context())
		if err != nil {
			t.Error(err)
		}
		entered <- end
	}()
	select {
	case <-entered:
		t.Fatal("Finish entered while an assumer that saw the gate closed runs")
	default:
	}
	release()
	end := <-entered

	// While the Finish runs, the gate is open: no assumer, and no close,
	// from the epoch before it or the one it opened.
	e2, closed := g.load()
	if closed {
		t.Fatal("gate closed while a Finish runs")
	}
	if ok, _ := g.assume(); ok {
		t.Fatal("assume while a Finish runs succeeded")
	}
	g.close(e)
	g.close(e2)
	if _, closed := g.load(); closed {
		t.Fatal("close while a Finish runs closed the gate")
	}

	// Its end moves the epoch, so a count read during it cannot close the
	// gate; one read after it can.
	end()
	g.close(e2)
	if _, closed := g.load(); closed {
		t.Fatal("close from an epoch a Finish ended closed the gate")
	}
	e3, _ := g.load()
	g.close(e3)
	if _, closed := g.load(); !closed {
		t.Fatal("close after the Finish ended left the gate open")
	}
}
