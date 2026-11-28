package kvtx

import (
	"context"
	"runtime"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
)

// TestLivenessRunThrough stores a tree, swaps the root to a second tree that
// shares a subtree, collects, and checks that the grace period keeps the old
// tree, that afterwards only what the live root reaches is stored, and that a
// hole under a dead page frees nothing below the page while the audit reports
// the hole.
func TestLivenessRunThrough(t *testing.T) {
	// Two stages opening together on a fresh Volume both find no process
	// lease, and the process starts exactly one: a second would leave the
	// roots of the first to lapse while their stages hold them.
	for range 16 {
		requireOneLease(t, openStagesTogether(t, 2))
	}

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

	// After the grace period, the old tree's own blocks go and the shared
	// subtree stays.
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

	// Releasing side frees R3, and the release of its children stops at the
	// hole: H stays, and the audit reports it leaked.
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

	// A released chain frees whole in one pass, though its releases queue
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
	// says. The index pass reads each young log page once, so only its
	// longest transaction is bounded: a net slice applies at most the
	// collector's 4,096 log keys, each costing at most its page's delete, a
	// count write and a queue write, whatever the Volume holds.
	const indexOps = 3*4096 + 64
	small, large := measureLivenessPaths(t, 300), measureLivenessPaths(t, 3000)
	within := func(l, s uint64) bool { return l <= s+s/4+4 }
	for name, s := range small.counts {
		l := large.counts[name]
		t.Logf("%s: %+v over 1x, %+v over 10x", name, s, l)
		bounded := within(l.Total(), s.Total()) && l.Txns <= s.Txns+1 && within(l.MaxWriteOps, s.MaxWriteOps)
		if name == "index" {
			bounded = l.MaxWriteOps <= indexOps
		}
		if !bounded {
			t.Errorf("%s over 10x stored blocks cost %+v, over 1x %+v", name, l, s)
		}
	}

	// A commit of three blocks reads each block's child list, bytes and count
	// record once, beyond the state and the root it moves. The count read lets
	// a put repark a block whose lost list left a zero count.
	if c := small.counts["commit"]; c.Reads+c.Steps > 3*3+4 {
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

	// A page replaced by one sharing its children holds them until its
	// free, so nothing cancels: each shared child costs exactly one write
	// in the net, for the new page's up, and one in the free, for the old
	// page's down. Each step reads the children's counts in one batch, so
	// the step's reads do not grow with the number of children.
	const perChild = 1
	net31, free31 := measureReplacedPage(t, 31)
	for _, shared := range []int{63, 127} {
		net, free := measureReplacedPage(t, shared)
		t.Logf("replaced page of %d shared children: net %d reads %d writes, free %d reads %d writes; of 31: net %d/%d, free %d/%d",
			shared, net.Reads+net.Steps, net.Writes, free.Reads+free.Steps, free.Writes,
			net31.Reads+net31.Steps, net31.Writes, free31.Reads+free31.Steps, free31.Writes)
		more := uint64(perChild * (shared - 31))
		if net.Reads+net.Steps != net31.Reads+net31.Steps || net.Writes != net31.Writes+more ||
			free.Reads+free.Steps != free31.Reads+free31.Steps || free.Writes != free31.Writes+more {
			t.Errorf("a replaced page of %d shared children cost other than the same reads and %d more writes than one of 31", shared, more)
		}
	}

	// A staged batch's lookups keep no commit waiting, at 3 or 32 children an
	// entry.
	if s, l := measureStageLookupWait(t, 3), measureStageLookupWait(t, 32); l > 2*s+time.Millisecond {
		t.Errorf("commits beside a staged batch of 32 children an entry waited %v at the second longest, of 3 %v", l, s)
	} else {
		t.Logf("stage lookups: second longest commit %v beside 3 children an entry, %v beside 32", s, l)
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

// measureLivenessPaths opens a Volume that frees at once over a counting store
// holding n stored blocks under roots, and returns what each entry point the
// Phase 1 path walk describes cost on it: a commit of a small tree with its
// root move while proofs of n/100 roots were marked, a mark of three proofs, a
// read of a block and its child list, a reader pin, a staged put, a cache fill,
// a collector pass over 2n young log entries and the pass after it, which
// collects the commit's tree, a put through a stage that holds n tops, and a
// commit beside 10n pins and 10n open stages.
func measureLivenessPaths(t *testing.T, n int) livenessCosts {
	// Open the Volume over a counting store.
	t.Helper()
	ctx := t.Context()
	cs := kvtx_kvtest.NewCountingStore(newGuardedStore(t))
	v, err := NewVolume(ctx, "test/scaling", store_kvkey.NewDefaultKVKey(), cs, &store_kvtx.Config{CollectorFrees: true}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	clk := &testClock{now: time.Unix(1_800_000_000, 0)}
	setVolumeClock(t, v, clk.Now)
	observePuts(v)
	t.Cleanup(func() { _ = v.Close() })

	// Store n leaves, a hundred under each parent and each parent under a
	// root, and net their logs.
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
			_, _, err := v.landFill(ctx, p, "b", tr.entries("F"))
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
	// The collect pass comes once the fill's lease lapsed, so the pass
	// releases F.
	clk.Add(fillLapse - block_gc.DefaultGrace/2)
	move(4)
	measure("collect", func() error {
		collectAll(t, v)
		return nil
	})

	// The tree ripens at the up log's end its release netted at, so it
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

// measureWideFree stores a block referencing children leaves under a root,
// releases it, and returns the most operations one transaction of the
// collection that frees it made, and the longest that transaction held the
// store's writer, by a collector that finds a writer waiting when waiting is
// set.
func measureWideFree(t *testing.T, children int, waiting bool) (uint64, time.Duration) {
	// Store the block and its leaves, then release it.
	t.Helper()
	ctx := t.Context()
	cs := kvtx_kvtest.NewCountingStore(newGuardedStore(t))
	v, err := NewVolume(ctx, "test/wide-free", store_kvkey.NewDefaultKVKey(), cs, &store_kvtx.Config{CollectorFrees: true}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	clk := &testClock{now: time.Unix(1_800_000_000, 0)}
	setVolumeClock(t, v, clk.Now)
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
		opts.WritersWaiting = func() bool { return true }
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

// measureReplacedPage stores a page referencing shared leaves and one leaf of
// its own under a root, nets it, then moves the root onto a new page
// referencing the same shared leaves and another leaf of its own, and returns
// what netting that commit and then freeing the old page cost the store.
func measureReplacedPage(t *testing.T, shared int) (net, free kvtx_kvtest.Counts) {
	// Store the page under main and net it.
	t.Helper()
	ctx := t.Context()
	cs := kvtx_kvtest.NewCountingStore(newGuardedStore(t))
	v, err := NewVolume(ctx, "test/replaced-page", store_kvkey.NewDefaultKVKey(), cs, &store_kvtx.Config{CollectorFrees: true}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	clk := &testClock{now: time.Unix(1_800_000_000, 0)}
	setVolumeClock(t, v, clk.Now)
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
// second longest of twenty changing commits made meanwhile.
func measureStageLookupWait(t *testing.T, children int) time.Duration {
	// Build a batch the stage would put.
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
			_, _ = v.stageHeld(t.Context(), stage, newStageNames(stage, entries))
		}
	}()

	// Commit a root move twenty times meanwhile, timing each.
	tr := testTree{}
	var prev *block.BlockRef
	waits := make([]time.Duration, 0, 20)
	for i := range 20 {
		name := "c" + strconv.Itoa(i)
		ref := tr.add(t, name)
		start := time.Now()
		if err := v.SetBucketRoots(ctx, "b", tr.entries(name), []block.NamedRoot{{Name: "c", Ref: ref, Previous: prev}}); err != nil {
			t.Fatal(err)
		}
		waits = append(waits, time.Since(start))
		prev = ref
	}
	close(done)
	<-looked

	// Return the second longest: one preemption or GC pause on a loaded host
	// lands on a single commit, while a commit that waited on the lookups
	// would wait in most of them, the lookups running back to back.
	slices.Sort(waits)
	return waits[len(waits)-2]
}

// TestLivenessAdversarial tries to make the collector free a live block: a
// reader pinning an old root during a swap and a reader whose lease lapses,
// two writers moving one root, a root moved away and back within the grace
// period, a release and a birth that share a child in different intervals,
// and a Volume that predates adoption. It frees no live block in any.
func TestLivenessAdversarial(t *testing.T) {
	testLivenessLeases(t)
	testLivenessAudit(t)
	testLivenessAdoptionFill(t)
	testLivenessCollection(t)
	testLivenessRelease(t)
	testLivenessLand(t)
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
			t.Fatalf("freed %d netted %d, want 0 freed and the logs netted", res.Freed, res.Netted)
		}
		requireStored(t, v, tr, true, "R1")
	})

	testLivenessAdoption(t)
}

// TestLivenessSettle walks the sequences in which a block a holder reaches
// references a child whose list a format 6 store cut short, so no holder counts
// the block below that list: a put again and a fill while the unmarked run
// builds, fills top down and under a held block, and a refilled hole. In each
// the settle keeps the block below while a root reaches it. It then checks each
// barrier a Land keeps and each clause the flip waits on, and that the sweep
// deletes exactly the old blocks nothing reaches.
func TestLivenessSettle(t *testing.T) {
	runLivenessSettle(t)
}
