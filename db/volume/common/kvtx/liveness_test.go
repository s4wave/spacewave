package kvtx

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"maps"
	"math"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"google.golang.org/protobuf/encoding/protowire"
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
func openLivenessVolume(t *testing.T, store kvtx.Store, frees bool) (*Volume, *testClock) {
	t.Helper()
	return openConfVolume(t, store, &store_kvtx.Config{CollectorFrees: frees})
}

// openConfVolume opens a Volume over store under conf.
func openConfVolume(t *testing.T, store kvtx.Store, conf *store_kvtx.Config) (*Volume, *testClock) {
	t.Helper()
	clk := &testClock{now: time.Unix(1_800_000_000, 0)}
	v, err := NewVolume(t.Context(), "test/liveness", store_kvkey.NewDefaultKVKey(), store, conf, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	v.clock = clk.Now
	auditCollector(t, v, conf.GetCollectorFrees())
	t.Cleanup(func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	})
	return v, clk
}

// observePuts lets v store blocks nothing holds, counting them as a Volume
// with frees off does, for a scenario that leaves garbage on purpose: data a
// Volume stored before it refused unheld puts, or a writer that dies before
// its root lands.
func observePuts(v *Volume) {
	v.heldMode = block_gc.HeldObserve
}

// auditCollector gives v a collector, frees on when frees is set, that checks
// requireFront after each of its transactions. A Volume without a collector
// keeps none.
func auditCollector(t *testing.T, v *Volume, frees bool) {
	t.Helper()
	if v.collector == nil {
		return
	}
	audited := &paceProbe{Store: collectorStore{v: v}, limit: math.MaxInt, after: func() { requireFront(t, v) }}
	swapCollector(v, v.newCollector(audited, frees, 0))
}

// swapCollector replaces the collector v opened with. A Volume reads its
// frees once at open, so a collector that frees also clears hidesNone, as an
// open with frees on would have left it.
func swapCollector(v *Volume, c *block_gc.Collector) {
	v.collector = c
	if c.Frees() {
		v.hidesNone.Store(false)
	}
}

// openUnadoptedVolume opens a Volume over store, which predates adoption,
// with the collector's frees on and no background adoption, so the test
// drives the adoption itself.
func openUnadoptedVolume(t *testing.T, store kvtx.Store) (*Volume, *testClock) {
	t.Helper()
	v, clk := openLivenessVolume(t, store, false)
	auditCollector(t, v, true)
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
		total.Lapsed += res.Lapsed
		total.Underflows += res.Underflows
		total.Unmarked += res.Unmarked
		total.Popped += res.Popped
		total.Seeded += res.Seeded
		total.Swept += res.Swept
		total.Flipped = total.Flipped || res.Flipped
		total.Unadopted = total.Unadopted || res.Unadopted
		total.Settled = total.Settled || res.Settled
		if !res.More {
			return total
		}
	}
	t.Fatal("collection never finished")
	return total
}

// netAll nets the logs to their ends with frees off, so each block the
// netting brings to zero stays on the free queue for a Land to take off.
func netAll(t *testing.T, v *Volume) block_gc.CollectResult {
	t.Helper()
	c := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{Now: v.now})
	var total block_gc.CollectResult
	for range 100 {
		res, err := c.Collect(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		total.Netted += res.Netted
		total.Queued += res.Queued
		if !res.More {
			return total
		}
	}
	t.Fatal("netting never finished")
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
	if res.LiveQueued != 0 || res.Leaked != leaked || res.Undercount != 0 || res.Unlisted != 0 {
		t.Fatalf("audit live queued %d (sample %v) leaked %d undercount %d (sample %v) unlisted %d, want 0, %d, 0 and 0",
			res.LiveQueued, res.LiveQueuedSample, res.Leaked, res.Undercount, res.UndercountSample, res.Unlisted, leaked)
	}
}

// requireListsStored checks that no stored block of tr lists a child that is
// not stored: no child is freed while a stored parent lists it, the model's NoLiveListsFreed.
func requireListsStored(t *testing.T, v *Volume, tr testTree) {
	t.Helper()
	stored := func(ref *block.BlockRef) bool {
		_, found, err := v.GetBlock(t.Context(), ref)
		if err != nil {
			t.Fatal(err)
		}
		return found
	}
	for name, e := range tr {
		if !stored(e.Ref) {
			continue
		}
		for _, child := range e.Refs {
			if !stored(child) {
				t.Fatalf("block %s stored over a freed child", name)
			}
		}
	}
}

// requireReads checks that each named block of tr reads its own bytes.
func requireReads(t *testing.T, v *Volume, tr testTree, names ...string) {
	t.Helper()
	for _, name := range names {
		data, found, err := v.GetBlock(t.Context(), tr[name].Ref)
		if err != nil || !found || !bytes.Equal(data, tr[name].Data) {
			t.Fatalf("block %s found=%t err %v, want its bytes", name, found, err)
		}
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

	// Releasing side frees R3, and the release of its children stops at the hole: H stays,
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
	// free, so nothing cancels: each shared child costs exactly one write in
	// the net, for the new page's up, and one in the free, for the old
	// page's down. Each reads the children's counts in one batch, so its
	// reads do not grow with them.
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
// fill, a collector pass over 2n young log entries and the pass after it,
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
			_, err := v.landFill(ctx, p, "b", tr.entries("F"))
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

// errMidRelease is the failure stopMidRelease returns.
var errMidRelease = errors.New("stopped mid-release")

// stopMidRelease runs a collector's transactions until one leaves a paged
// release in progress, the up pages from pages[0] to pages[1] partly netted
// when pages is set, or until true when until is set, then fails every later
// one, as a process that dies there.
type stopMidRelease struct {
	block_gc.Store
	// v is the Volume, whose state it reads after each transaction.
	v *Volume
	// pages, when its end is set, selects the up pages from its start up to
	// its end partly netted instead.
	pages [2]uint64
	// until, when set, selects the first transaction after which it reports
	// true of the state instead.
	until func(state *block_gc.LivenessState) bool
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
		s.stopped = state.GetRelease() != nil
		if cursor := state.GetUpCursor(); s.pages[1] != 0 {
			s.stopped = cursor > s.pages[0] && cursor < s.pages[1]
		}
		if s.until != nil {
			s.stopped = s.until(state)
		}
		return err
	})
}

// livenessState returns the liveness state of v.
func livenessState(t *testing.T, v *Volume) *block_gc.LivenessState {
	t.Helper()
	var state *block_gc.LivenessState
	err := v.readRecords(t.Context(), func(rec *block_gc.Records) error {
		var err error
		state, err = rec.GetState(t.Context())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
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
			_, _ = v.stageHeld(t.Context(), stage, newStageNames(stage, entries))
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

// writeOlderState rewrites the liveness state in store as a build that
// predates the frees switch does on a state write: it reads the record into a
// message without fields 15 and 16, keeps their bytes as unknown fields after
// its own, and moves up_seq.
func writeOlderState(t *testing.T, store kvtx.Store) {
	t.Helper()
	known, unknown := splitStateFields(t, store)
	if len(unknown) == 0 {
		t.Fatal("the state records no frees switch")
	}
	state := &block_gc.LivenessState{}
	if err := state.UnmarshalVT(known); err != nil {
		t.Fatal(err)
	}
	state.UpSeq++
	older, err := state.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	setStateValue(t, store, append(older, unknown...))
}

// splitStateFields reads the liveness state in store and splits its encoding
// into the fields a build older than the frees switch knows and the bytes of
// fields 15 and 16, the switch and its mark, which it does not.
func splitStateFields(t *testing.T, store kvtx.Store) (known, unknown []byte) {
	t.Helper()
	ctx := t.Context()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	value, found, err := tx.Get(ctx, livenessStateKey)
	if err != nil || !found {
		t.Fatalf("read the state: found %v err %v", found, err)
	}
	for b := value; len(b) != 0; {
		num, _, n := protowire.ConsumeField(b)
		if n < 0 {
			t.Fatal(protowire.ParseError(n))
		}
		if num == 15 || num == 16 {
			unknown = append(unknown, b[:n]...)
		} else {
			known = append(known, b[:n]...)
		}
		b = b[n:]
	}
	return known, unknown
}

// setStateValue writes value as the liveness state in store.
func setStateValue(t *testing.T, store kvtx.Store, value []byte) {
	t.Helper()
	ctx := t.Context()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, livenessStateKey, value); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// livenessStateKey is the store key of the liveness state record.
var livenessStateKey = []byte{livenessPrefix[0], 'S'}

// loseBlock loses ref from the store with its child list but keeps its count,
// as damage no audit has seen yet does.
func loseBlock(t *testing.T, v *Volume, ref *block.BlockRef) {
	t.Helper()
	err := v.withDirectAtomic(t.Context(), func(p *physicalTx) error {
		if err := p.rec.DeleteChildren(t.Context(), ref); err != nil {
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

	t.Run("reader sight across an adoption", func(t *testing.T) {
		// A reader sees head R1 of main, which then moves to R2, and the
		// Volume adopts again before the reader pins R1: the adoption drops
		// the record of the move and counts R1 at zero.
		v, clk := newLivenessVolume(t, true)
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
		markUnadopted(t, v)
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}

		// A pass at once frees nothing the reader saw: the adoption's marker
		// record is not Grace old, so R1 has not ripened. The reader pins R1
		// within its window and reads the whole tree past the grace period.
		collectAll(t, v)
		requireStored(t, v, tr, true, "R1", "E", "R2")
		unpin, err := v.PinBucketRoot(ctx, r1)
		if err != nil {
			t.Fatal(err)
		}
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R1", "E", "R2")

		// Once the pin is released, the old tree goes after the grace
		// period.
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
			// behind more commits than one net slice holds. The pass once the
			// first move's down ages nets the move back's up first, so R1
			// never reaches zero. The churn root holds X alone, so Y is
			// stored unheld.
			v, clk := newLivenessVolume(t, true)
			observePuts(v)
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

			// Net the first move's down: R1 keeps the move back's holder.
			clk.Add(block_gc.DefaultGrace/2 + time.Second)
			if res := collectAll(t, v); res.Freed != 0 {
				t.Fatalf("freed %d, want R1 kept by the move back", res.Freed)
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
		// names F under another root. The pass once the release ages nets
		// the birth's up first, so freeing R1 and C leaves F its new
		// holder.
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

		// Net the release: R1 and C go, and F keeps its new holder.
		clk.Add(2 * time.Second)
		if res := collectAll(t, v); res.Freed != 2 {
			t.Fatalf("freed %d, want R1 and C", res.Freed)
		}
		requireStored(t, v, tr, false, "R1", "C")
		clk.Add(block_gc.DefaultGrace)
		collectAll(t, v)
		requireStored(t, v, tr, true, "F", "R2")
		requireAudit(t, v, 0)
	})

	t.Run("re-put of a queued block", func(t *testing.T) {
		// R -> X loses main, and just inside its grace period a writer puts X
		// again, as a writer that puts one tree across several batches does
		// for a block it already stored. The pass once the release ages
		// nets the re-put's up first, so freeing R leaves X the re-put's
		// holder, and a parent the writer puts later still finds X.
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

		// Net the release: R goes and X stays.
		clk.Add(2 * time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R")
		requireStored(t, v, tr, true, "X")

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
		// X loses main and nets with frees off, so it is at zero on the free
		// queue. A writer then puts X again and, just inside the grace
		// period, a parent naming it under main. The put again takes X off
		// the free queue in its own Land, and its up lifts X off zero once
		// it nets, so no pass frees X before the parent's up nets.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		x := tr.add(t, "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X"), []block.NamedRoot{{Name: "main", Ref: x}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: x}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := netAll(t, v); res.Queued != 1 {
			t.Fatalf("queued %d, want X", res.Queued)
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

		// No pass frees X: first the put again holds it, then the parent.
		clk.Add(2 * time.Second)
		if res := collectAll(t, v); res.Freed != 0 {
			t.Fatalf("freed %d of a block put again", res.Freed)
		}
		clk.Add(block_gc.DefaultGrace)
		collectAll(t, v)
		requireStored(t, v, tr, true, "X", "P")
		requireAudit(t, v, 0)
	})

	t.Run("proof of a released block", func(t *testing.T) {
		// R -> C is proved complete, then loses main and nets with frees
		// off, so R is Dead and stored. Its
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

		// Release R and net it with frees off, so R is Dead and queued.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		netAll(t, v)
		requireStored(t, v, tr, true, "R", "C")

		// R holds C until R itself is freed, so its proof stays true and a
		// stage may hold R on it.
		if !proved() {
			t.Fatal("released R lost its proof before its free")
		}

		// A pending proof of R counts for nothing, nor holds it under a stage,
		// and carrying it writes none; the recorded proof it rides over
		// stays.
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
		if !proved() {
			t.Fatal("carrying a proof of a released R dropped the recorded one")
		}
		requireStored(t, v, tr, true, "R", "C")

		// Once R ripens, it and C go.
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R", "C")
		requireStored(t, v, tr, true, "Z")
		requireAudit(t, v, 0)
	})

	t.Run("root moved onto a Dead block", func(t *testing.T) {
		// R -> E loses main and nets with frees off, so R is Dead and on the
		// free queue. Root k then moves onto R without a put. The move's
		// Land takes R off the free queue, so no pass frees R or E before
		// the move nets and holds them again.
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
		clk.Add(block_gc.DefaultGrace + time.Second)
		netAll(t, v)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "k", Ref: r}}); err != nil {
			t.Fatal(err)
		}

		// The move dequeued R, so no pass frees it; once the move nets, R
		// and E are held again.
		if res := collectAll(t, v); res.Freed != 0 {
			t.Fatalf("freed %d of a chain a root moved onto", res.Freed)
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

	for _, tc := range []struct {
		name string
		mode sliceMode
	}{
		{"in slices with collection between slices", sliceCollect},
		{"in slices cancelled then retried", sliceCancelRetry},
		{"in slices cancelled then released", sliceCancelRelease},
		{"in slices released between slices", sliceRelease},
		{"parent first in one transaction", sliceParentFirst},
		{"in one transaction under StageOneTransaction", sliceOneTransaction},
	} {
		t.Run("stage lands "+tc.name, func(t *testing.T) {
			testStageSlices(t, tc.mode)
		})
	}

	t.Run("underflow in one free slice", func(t *testing.T) {
		// A -> B under main, with B also at zero as if a holder had vanished
		// from its count. Releasing main brings A to zero, and its free
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
				_, err := v.landFill(ctx, p, "b", tr.entries(name))
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
		// A read fills E, which nothing else holds. Landed with no root it is
		// refused; the fill's leased root holds it for the grace period, and
		// once the root lapses it goes after the grace period, as an unheld
		// put does.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E")
		err := v.withDirectFill(ctx, func(p *physicalTx) error {
			_, err := v.land(ctx, p, "", tr.entries("E"), nil)
			return err
		})
		if !errors.Is(err, block.ErrUnheldPut) {
			t.Fatalf("fill with no root returned %v, want ErrUnheldPut", err)
		}
		err = v.withDirectFill(ctx, func(p *physicalTx) error {
			_, err := v.landFill(ctx, p, "b", tr.entries("E"))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		collectAll(t, v)
		requireStored(t, v, tr, true, "E")
		requireAudit(t, v, 0)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
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
			if err := v.MarkRootsComplete(ctx, []*block.BlockRef{tr["T"].Ref}); err != nil {
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

			// The check a stage copy makes before its release, with no write
			// through the stage since, finds T's proof gone and holds nothing.
			if held, err := v.HoldStageCompleteRoots(ctx, stage, []*block.BlockRef{tr["T"].Ref}); err == nil && held[0] {
				t.Fatal("the stage check held a graph the collector freed")
			}
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
			// the hole, the put counts H as stored under R, so H stays at
			// exactly R's holder, through a second refill as well.
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
			refill := func() {
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
			}
			refill()
			if !audited {
				if res, err := v.Audit(ctx); err != nil || res.Undercount != 1 {
					t.Fatalf("audit of an unrecorded refill found %d undercounts err %v, want 1", res.Undercount, err)
				}
				return
			}
			for i := range 2 {
				clk.Add(block_gc.DefaultGrace + time.Second)
				collectAll(t, v)
				requireStored(t, v, tr, true, "R", "H")
				if holders, _ := blockHolders(t, v, tr, "H"); holders != 1 {
					t.Fatalf("after refill %d H has %d holders, want R's alone", i+1, holders)
				}
				requireAudit(t, v, 0)
				if i == 0 {
					refill()
				}
			}
		})
	}

	for name, lapsed := range map[string]bool{"live": false, "lapsed": true} {
		t.Run("stage parent ahead of its child "+name, func(t *testing.T) {
			// A stage stores P, which names C, while C waits in the writer's
			// memory, and its process dies before C lands. The audit records
			// C as a hole either way. Only once the stage's lease lapsed does
			// it count C as a hole no live holder names.
			v, clk := newLivenessVolume(t, false)
			ctx := t.Context()
			tr := testTree{}
			tr.add(t, "C")
			tr.add(t, "P", "C")
			stage, _, err := v.OpenStage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("P")); err != nil {
				t.Fatal(err)
			}
			want := uint64(0)
			if lapsed {
				clk.Add(rootLease + time.Second)
				want = 1
			}
			res, err := v.Audit(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if res.Holes != 1 || res.LapsedHoles != want || uint64(len(res.Mark.LapsedRefs)) != want {
				t.Fatalf("audit recorded %d holes, %d of them lapsed, and found %d lapsed, want 1, %d and %d",
					res.Holes, res.LapsedHoles, len(res.Mark.LapsedRefs), want, want)
			}
		})
	}

	t.Run("streaming audit beside writers", func(t *testing.T) {
		testAuditWalk(t)
	})

	t.Run("streaming audit under stalled netting", func(t *testing.T) {
		testAuditWalkStalled(t)
	})

	for _, tc := range []struct {
		name           string
		audited, named bool
	}{
		{"unaudited", false, false},
		{"audited", true, false},
		{"audited and named again", true, true},
	} {
		t.Run("stage dies holding a parent before its child "+tc.name, func(t *testing.T) {
			// An adopted format 6 store, its generation Open, holds the old
			// root A. A live stage holds L. A second stage puts P, which
			// names C, before C, as a buffered store's drain may order
			// them, and its process dies with both unput C and P's lapsing
			// root. An audit while the root lives records C as a hole.
			// Past the lease the collector releases the dead stage's root
			// and keeps L. Unaudited, P frees once the generation settles.
			// Audited, the hole record outlives every block that named C
			// and holds the flip: the generation stays Open, P stays
			// parked, and no pass reports why. A commit that names P again
			// after the lapse, and later puts C, refills the hole, keeps
			// both, and lets the generation settle.
			store := store_kvtx_inmem.NewStore()
			tr := testTree{}
			tr.add(t, "A")
			writeLegacyStore(t, store, store_kvkey.NewDefaultKVKey(), tr, "A", true, "A")
			v, clk := openUnadoptedVolume(t, store)
			ctx := t.Context()
			if _, err := v.Adopt(ctx); err != nil {
				t.Fatal(err)
			}
			if gen := livenessGeneration(t, v); gen != block_gc.Generation_GENERATION_OPEN {
				t.Fatalf("the adoption left the generation %v, want it open", gen)
			}
			live, releaseLive, err := v.OpenStage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseLive()
			tr.add(t, "L")
			if _, err := v.PrepareStagedBlockBatch(ctx, live, tr.entries("L")); err != nil {
				t.Fatal(err)
			}

			// The dying stage puts P alone; its process stops renewing.
			dead, _, err := v.OpenStage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			tr.add(t, "C")
			p := tr.add(t, "P", "C")
			if _, err := v.PrepareStagedBlockBatch(ctx, dead, tr.entries("P")); err != nil {
				t.Fatal(err)
			}
			unlock, err := v.rootPinMu.Lock(ctx)
			if err != nil {
				t.Fatal(err)
			}
			delete(v.stages, dead)
			unlock()
			if tc.audited {
				if res, err := v.Audit(ctx); err != nil || res.Holes != 1 {
					t.Fatalf("audit under the live stage root recorded %d holes err %v, want C", res.Holes, err)
				}
			}

			// Past the lease the dead stage's root lapses, the live one's
			// does not.
			advanceRenewing(t, v, clk, rootLease+time.Second)
			if res := collectAll(t, v); res.Lapsed != 1 {
				t.Fatalf("pass past the lease lapsed %d roots, want the dead stage's", res.Lapsed)
			}
			if tc.named {
				if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
					t.Fatal(err)
				}
				if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), nil); err != nil {
					t.Fatal(err)
				}
			}

			// Settle as far as the store lets it.
			for range 8 {
				if livenessGeneration(t, v) == block_gc.Generation_GENERATION_OPEN {
					walkFront(t, v, store, tr)
				}
				advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
				collectAll(t, v)
			}
			requireReads(t, v, tr, "A", "L")
			gen := livenessGeneration(t, v)
			holes := countPrefix(t, store, slices.Concat(livenessPrefix, []byte{'O'}))
			switch {
			case tc.named:
				requireReads(t, v, tr, "P", "C")
				if gen != block_gc.Generation_GENERATION_SETTLED || holes != 0 {
					t.Fatalf("after the refill the generation is %v with %d hole records, want Settled and none", gen, holes)
				}
			case tc.audited:
				requireStored(t, v, tr, true, "P")
				requireStored(t, v, tr, false, "C")
				if gen != block_gc.Generation_GENERATION_OPEN || holes != 1 {
					t.Fatalf("the generation is %v with %d hole records, want Open, held by C's", gen, holes)
				}
			default:
				requireStored(t, v, tr, false, "P", "C")
				if gen != block_gc.Generation_GENERATION_SETTLED || holes != 0 {
					t.Fatalf("the generation is %v with %d hole records, want Settled and none", gen, holes)
				}
			}
			requireAudit(t, v, 0)
		})
	}

	t.Run("cache fill during an adoption", func(t *testing.T) {
		// main holds C on an unadopted Volume. While an adoption settles its
		// counts, after it counted the child lists, a cache fill stores F
		// naming C, logging an up for C and a down that takes F's implied
		// holder back. The adoption treats F as born and leaves it to the
		// logs. A root r then moves onto F: F stays while r holds it, and
		// once r is released F goes and C stays under main.
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
		if _, err := block_gc.Adopt(ctx, probe, v.now); err != nil || len(probe.at) != 0 {
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
						_, err := v.landFill(ctx, px, "b", tr.entries("B"))
						return err
					})
					if err != nil {
						t.Error(err)
					}
				},
			}}
			if _, err := block_gc.Adopt(ctx, probe, v.now); err != nil || len(probe.at) != 0 {
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
	})

	t.Run("cache fill of a held block during an adoption", func(t *testing.T) {
		// main holds P on an unadopted Volume, and P names B, which the
		// Volume lacks. While an adoption runs, before it settles, a read
		// fills B. The adoption counts P's holder of B and leaves B to the
		// fill's logs, so after the grace period B keeps P's holder.
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
					_, err := v.landFill(ctx, px, "b", tr.entries("B"))
					return err
				})
				if err != nil {
					t.Error(err)
				}
			},
		}}
		if _, err := block_gc.Adopt(ctx, probe, v.now); err != nil || len(probe.at) != 0 {
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
		// of H's leaves, an up for each. A waiting writer does not shrink the
		// netting slices, so they reach the up log's end, and the writer's
		// ups delay no free: G and its leaves are freed in a bounded number
		// of transactions while the writer's blocks stay.
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
			Frees:          true,
			Now:            v.now,
			WritersWaiting: func() bool { return true },
		}
		if freed := collectThrough(t, probe, opts); freed != 101 {
			t.Fatalf("freed %d of G and its 100 leaves", freed)
		}
		requireStored(t, v, tr, false, append(slices.Clone(leaves), "G")...)
		requireStored(t, v, tr, true, held...)
		requireAudit(t, v, 0)
	})

	t.Run("revive over a freed child", func(t *testing.T) {
		// main holds R2 -> C2 -> L and moves to R1, and a pass with frees
		// off nets the move, so R2 is at zero on the free queue when main
		// moves back onto R2 with no put. No transaction frees a child while
		// a stored parent lists it, so the revived tree reads.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "L")
		tr.add(t, "C2", "L")
		r2 := tr.add(t, "R2", "C2")
		r1 := tr.add(t, "R1")
		z := tr.add(t, "Z")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("L", "C2", "R2", "Z"), []block.NamedRoot{{Name: "main", Ref: r2}, {Name: "z", Ref: z}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R1"), []block.NamedRoot{{Name: "main", Ref: r1, Previous: r2}}); err != nil {
			t.Fatal(err)
		}
		probe := &paceProbe{Store: collectorStore{v: v}, limit: 10_000, after: func() {
			requireListsStored(t, v, tr)
		}}
		opts := block_gc.CollectorOptions{Frees: true, Now: v.now}
		clk.Add(block_gc.DefaultGrace + time.Second)
		netAll(t, v)

		// Revive R2 with no put; R1 goes, and R2's tree reads.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		var freed uint64
		for range 3 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			freed += collectThrough(t, probe, opts)
		}
		if freed != 1 {
			t.Fatalf("freed %d after the revive, want R1 alone", freed)
		}
		requireReads(t, v, tr, "R2", "C2", "L", "Z")
		requireStored(t, v, tr, false, "R1")
		requireAudit(t, v, 0)

		// Once main leaves R2, its whole tree goes.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r2}}); err != nil {
			t.Fatal(err)
		}
		freed = 0
		for range 4 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			freed += collectThrough(t, probe, opts)
		}
		if freed != 3 {
			t.Fatalf("freed %d of R2's tree, want 3", freed)
		}
		requireStored(t, v, tr, false, "R2", "C2", "L")
		requireAudit(t, v, 0)
	})

	t.Run("shared child at its other parent's free", func(t *testing.T) {
		// C is under R1, which main holds, and under R3, whose root x is
		// released and netted to zero but not freed. main then moves from R1
		// onto R3 in one Land, and the next pass frees R1. C keeps R3's
		// holder through R1's free.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		r1 := tr.add(t, "R1", "C")
		r3 := tr.add(t, "R3", "C")
		roots := []block.NamedRoot{{Name: "main", Ref: r1}, {Name: "x", Ref: r3}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C", "R1", "R3"), roots); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "x", Previous: r3}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		nets := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{Now: v.now})
		if res, err := nets.Collect(ctx); err != nil || res.Netted == 0 || res.Freed != 0 {
			t.Fatalf("netting x's release = %+v err %v, want it netted and nothing freed", res, err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: r3, Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		probe := &paceProbe{Store: collectorStore{v: v}, limit: 10_000, after: func() {
			requireListsStored(t, v, tr)
		}}
		opts := block_gc.CollectorOptions{Frees: true, Now: v.now}
		var freed uint64
		for range 3 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			freed += collectThrough(t, probe, opts)
		}
		if freed != 1 {
			t.Fatalf("freed %d, want R1 alone", freed)
		}
		requireReads(t, v, tr, "R3", "C")
		requireStored(t, v, tr, false, "R1")
		requireAudit(t, v, 0)
	})

	t.Run("deep tree under steady writes", func(t *testing.T) {
		// A tree five levels deep, each block naming two, loses its root,
		// and a writer commits a new root naming A, which root a holds,
		// after every collector transaction, an up pair for each. The
		// writer's ups net with the tree's release, and each free's child
		// downs queue at the up log's end, so the whole tree of 31 blocks
		// goes in the first pass past the grace period, with exact counts,
		// and the writer's blocks stay.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		var level []string
		for i := range 16 {
			name := "level 5 block " + strconv.Itoa(i)
			tr.add(t, name)
			level = append(level, name)
		}
		names := slices.Clone(level)
		for depth := 4; depth >= 1; depth-- {
			var up []string
			for i := 0; i < len(level); i += 2 {
				name := "level " + strconv.Itoa(depth) + " block " + strconv.Itoa(i/2)
				tr.add(t, name, level[i], level[i+1])
				up = append(up, name)
			}
			names = append(names, up...)
			level = up
		}
		root := tr[level[0]].Ref
		a := tr.add(t, "A")
		roots := []block.NamedRoot{{Name: "tree", Ref: root}, {Name: "a", Ref: a}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(names), "A")...), roots); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "tree", Previous: root}}); err != nil {
			t.Fatal(err)
		}
		held := []string{"A"}
		probe := &paceProbe{Store: collectorStore{v: v}, limit: 10_000, after: func() {
			requireListsStored(t, v, tr)
			name := "write " + strconv.Itoa(len(held))
			ref := tr.add(t, name, "A")
			if err := v.SetBucketRoots(ctx, "b", tr.entries(name), []block.NamedRoot{{Name: name, Ref: ref}}); err != nil {
				t.Fatal(err)
			}
			held = append(held, name)
		}}
		opts := block_gc.CollectorOptions{Frees: true, Now: v.now}
		for i, n := range []uint64{31} {
			clk.Add(block_gc.DefaultGrace + time.Second)
			if freed := collectThrough(t, probe, opts); freed != n {
				t.Fatalf("round %d freed %d, want %d", i, freed, n)
			}
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if freed := collectThrough(t, probe, opts); freed != 0 {
			t.Fatalf("freed %d past the tree", freed)
		}
		requireStored(t, v, tr, false, names...)
		requireStored(t, v, tr, true, held...)
		requireAudit(t, v, 0)
	})

	t.Run("restart after a second process unadopts", func(t *testing.T) {
		// A, with frees on, adopts a Volume holding main -> R -> X and an
		// unheld G. B opens the same store writable with frees off, returns
		// it to unadopted, as its reset of an adoption in progress or a list
		// rebuild does, and closes. A restarts with frees on: its open
		// counts the Volume again, and G goes while R and X stay.
		store := store_kvtx_inmem.NewStore()
		a, _ := openLivenessVolume(t, store, true)
		observePuts(a)
		waitAdopted(t, a)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		tr.add(t, "G")
		if err := a.SetBucketRoots(ctx, "b", tr.entries("X", "R", "G"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
		b, _ := openLivenessVolume(t, store, false)
		markUnadopted(t, b)
		if err := b.Close(); err != nil {
			t.Fatal(err)
		}

		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireReads(t, v, tr, "R", "X")
		requireStored(t, v, tr, false, "G")
		requireAudit(t, v, 0)
	})

	t.Run("restart between turning frees on and the recount", func(t *testing.T) {
		// A Volume holding main -> R -> X and an unheld G turns frees on,
		// which returns it to unadopted, and closes before its recount
		// runs. Reopened with frees on, which writes nothing since the
		// switch is on already, it counts again before any free, and G
		// goes while R and X stay.
		store := store_kvtx_inmem.NewStore()
		v1, _ := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		tr.add(t, "G")
		if err := v1.SetBucketRoots(ctx, "b", tr.entries("X", "R", "G"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		state, err := block_gc.SetFrees(ctx, collectorStore{v: v1}, true)
		if err != nil {
			t.Fatal(err)
		}
		if !state.GetFrees() || state.GetAdoption() == block_gc.Adoption_ADOPTION_ADOPTED {
			t.Fatalf("turning frees on left the state %v, want frees on and a recount due", state)
		}
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}

		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireReads(t, v, tr, "R", "X")
		requireStored(t, v, tr, false, "G")
		requireAudit(t, v, 0)
	})

	t.Run("frees switched off and on", func(t *testing.T) {
		// A Volume born with frees on holds main -> R -> X and d -> D -> E,
		// and d lets D go. Restarted with frees off, it nets the release
		// and frees nothing, and records frees off. Restarted with them on
		// again, it returns to unadopted and counts every holder again
		// before its first free, then frees D and E and keeps R and X.
		store := store_kvtx_inmem.NewStore()
		v1, _ := openLivenessVolume(t, store, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		tr.add(t, "E")
		d := tr.add(t, "D", "E")
		roots := []block.NamedRoot{{Name: "main", Ref: r}, {Name: "d", Ref: d}}
		if err := v1.SetBucketRoots(ctx, "b", tr.entries("X", "R", "E", "D"), roots); err != nil {
			t.Fatal(err)
		}
		if err := v1.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "d", Previous: d}}); err != nil {
			t.Fatal(err)
		}
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}

		// Restart with frees off: the release nets and nothing frees.
		v2, clk2 := openLivenessVolume(t, store, false)
		var freed uint64
		for range 3 {
			clk2.Add(3 * (block_gc.DefaultGrace + time.Second))
			freed += collectAll(t, v2).Freed
		}
		if freed != 0 {
			t.Fatalf("freed %d with frees off, want 0", freed)
		}
		if state := livenessState(t, v2); state.GetFrees() {
			t.Fatal("an open with frees off left them recorded on")
		}
		requireStored(t, v2, tr, true, "X", "R", "E", "D")
		if err := v2.Close(); err != nil {
			t.Fatal(err)
		}

		// Restart with frees on: the switch recounts, then D and E go.
		v, clk := openLivenessVolume(t, store, true)
		if state := waitAdopted(t, v); !state.GetFrees() || state.GetAdoptionRun() == "" {
			t.Fatalf("frees turned on left the state %v, want them on and a recount's run", state)
		}
		freed = 0
		for range 3 {
			clk.Add(3 * (block_gc.DefaultGrace + time.Second))
			freed += collectAll(t, v).Freed
		}
		if freed != 2 {
			t.Fatalf("freed %d after frees turned on again, want D and E", freed)
		}
		requireReads(t, v, tr, "X", "R")
		requireStored(t, v, tr, false, "E", "D")
		requireAudit(t, v, 0)
	})

	t.Run("turning frees on recounts an adopted Volume", func(t *testing.T) {
		// An adopted Volume with frees off holds main -> R -> P -> C, and
		// the store loses C with its list but keeps its count, so no audit
		// records the hole. Turning frees on recounts, which counts C one
		// holder more than P, its namer, so a put of C alone leaves it held
		// by P and it outlives the grace.
		store := store_kvtx_inmem.NewStore()
		v1, _ := openLivenessVolume(t, store, true)
		waitAdopted(t, v1)
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}
		v2, _ := openLivenessVolume(t, store, false)
		ctx := t.Context()
		tr := testTree{}
		c := tr.add(t, "C")
		tr.add(t, "P", "C")
		r := tr.add(t, "R", "P")
		if err := v2.SetBucketRoots(ctx, "b", tr.entries("C", "P", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		loseBlock(t, v2, c)
		if err := v2.Close(); err != nil {
			t.Fatal(err)
		}

		// Turn frees on, put C alone, and collect past the grace.
		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), nil); err != nil {
			t.Fatal(err)
		}
		for range 3 {
			clk.Add(3 * (block_gc.DefaultGrace + time.Second))
			collectAll(t, v)
		}
		requireReads(t, v, tr, "R", "P", "C")
		if holders, _ := blockHolders(t, v, tr, "C"); holders != 1 {
			t.Fatalf("C counts %d holders after its put, want P's 1", holders)
		}
		requireAudit(t, v, 0)
	})

	t.Run("the audit sees the residual", func(t *testing.T) {
		// After the recount, the store loses C with its list, and a put of
		// C alone comes before any audit records the hole. That put leaves
		// C one holder short of P, its namer, the residual window, and an
		// audit run before the grace reports it. Nothing the Volume records
		// holds C, so the put stores it unheld.
		v, _ := newLivenessVolume(t, true)
		observePuts(v)
		waitAdopted(t, v)
		ctx := t.Context()
		tr := testTree{}
		c := tr.add(t, "C")
		tr.add(t, "P", "C")
		r := tr.add(t, "R", "P")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C", "P", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		loseBlock(t, v, c)
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), nil); err != nil {
			t.Fatal(err)
		}
		res, err := block_gc.Audit(ctx, collectorStore{v: v}, v.now(), false)
		if err != nil {
			t.Fatal(err)
		}
		if res.Undercount != 1 || !slices.ContainsFunc(res.UndercountSample, c.EqualsRef) {
			t.Fatalf("audit found %d undercounts in %v, want C alone", res.Undercount, res.UndercountSample)
		}
	})

	t.Run("turning frees on recounts after an older build writes", func(t *testing.T) {
		// A Volume adopted with frees on is written by a build that knows
		// neither the switch nor its mark, which keeps both as unknown fields
		// and moves next_seq. Opened again with frees on, the stale mark
		// reads as frees off, so the open returns the Volume to unadopted
		// and counts again before any free.
		store := store_kvtx_inmem.NewStore()
		v1, _ := openLivenessVolume(t, store, true)
		first := waitAdopted(t, v1)
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}
		writeOlderState(t, store)
		v, _ := openLivenessVolume(t, store, true)
		state := waitAdopted(t, v)
		if !state.GetFrees() || state.GetAdoptionRun() == "" || state.GetAdoptionRun() == first.GetAdoptionRun() {
			t.Fatalf("frees on after an older build's write left the state %v, want them on and a new recount's run", state)
		}
	})

	t.Run("turning frees on recounts what an older build wrote", func(t *testing.T) {
		// A Volume adopted with frees on names main -> R -> P -> C. A build
		// that predates the switch then lands R2 -> P, releases main and
		// leaves P one holder short, a count only a recount corrects. It
		// keeps the switch and its mark as unknown fields, so the mark goes
		// stale. Opened again with frees on, the Volume recounts before any
		// free: R2, P and C read after collections past the grace, and R,
		// which nothing names, frees.
		store := store_kvtx_inmem.NewStore()
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		p := tr.add(t, "P", "C")
		r := tr.add(t, "R", "P")
		r2 := tr.add(t, "R2", "P")
		v1, _ := openLivenessVolume(t, store, true)
		waitAdopted(t, v1)
		if err := v1.SetBucketRoots(ctx, "b", tr.entries("C", "P", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}
		_, switched := splitStateFields(t, store)

		// The older build's writes, made here by a frees-off open, with the
		// switch and its mark put back as that build keeps them.
		v2, clk2 := openLivenessVolume(t, store, false)
		if err := v2.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main2", Ref: r2}}); err != nil {
			t.Fatal(err)
		}
		if err := v2.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
			t.Fatal(err)
		}
		clk2.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v2)
		err := v2.withDirectAtomic(ctx, func(tx *physicalTx) error {
			holders, ripe, err := tx.rec.GetHolders(ctx, p)
			if err != nil {
				return err
			}
			if holders == 0 {
				return errors.New("P counts no holder")
			}
			return tx.rec.SetHolders(ctx, p, holders-1, ripe)
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := v2.Close(); err != nil {
			t.Fatal(err)
		}
		known, _ := splitStateFields(t, store)
		setStateValue(t, store, append(known, switched...))

		// Frees on again recounts, then collects.
		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		for range 4 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
		}
		requireReads(t, v, tr, "R2", "P", "C")
		requireStored(t, v, tr, false, "R")
		requireAudit(t, v, 0)
	})

	for _, tc := range []struct {
		who, ripe string
		restart   bool
	}{
		{"F again", "unripe", false}, {"F again", "unripe behind a record", false}, {"F again", "ripe", false},
		{"a new parent", "unripe", false}, {"a new parent", "unripe behind a record", false}, {"a new parent", "ripe", false},
		{"a new parent", "unripe behind a record", true},
	} {
		ripe, title := tc.ripe, "born put over children of an inline free "+tc.who+" "+tc.ripe
		if tc.restart {
			title += " across a collector restart"
		}
		t.Run(title, func(t *testing.T) {
			// F names three leaves, few enough to free inline, and root g
			// holds G. Root f releases F, and the collector stops once F is
			// freed with its leaves still stored. A writer then puts F again,
			// born since its bytes and list are gone, or a new parent P,
			// naming the leaves: right after the free, so its record is the
			// first after it; behind a younger root move onto G that nets
			// first; or after that move netted, so the leaves are ripe. The
			// same collector then runs on, its tail index extended from the
			// free, or with restart a new one, whose index is read whole from
			// the cursor. Every leaf the parent lists must still read once
			// the collector is quiet.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			leaves := []string{"leaf 0", "leaf 1", "leaf 2"}
			for _, name := range leaves {
				tr.add(t, name)
			}
			f := tr.add(t, "F", leaves...)
			np := tr.add(t, "P", leaves...)
			gr := tr.add(t, "G")
			roots := []block.NamedRoot{{Name: "f", Ref: f}, {Name: "g", Ref: gr}}
			if err := v.SetBucketRoots(ctx, "b", tr.entries(append(slices.Clone(leaves), "F", "G")...), roots); err != nil {
				t.Fatal(err)
			}
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "f", Previous: f}}); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			stored := func(name string) bool {
				_, found, err := v.GetBlock(ctx, tr[name].Ref)
				if err != nil {
					t.Fatal(err)
				}
				return found
			}
			// Stop once F is freed.
			resumed := false
			probe := &stopMidRelease{Store: collectorStore{v: v}, v: v, until: func(*block_gc.LivenessState) bool {
				return !resumed && !stored("F")
			}}
			opts := block_gc.CollectorOptions{Frees: true, Now: v.now}
			c := block_gc.NewCollector(probe, opts)
			res, err := c.Collect(ctx)
			if !errors.Is(err, errMidRelease) {
				t.Fatalf("collect = %+v err %v, want it stopped once F is freed", res, err)
			}
			freed := res.Freed
			left := 0
			for _, name := range leaves {
				if stored(name) {
					left++
				}
			}
			t.Logf("stopped with F stored %t and %d of %d leaves stored", stored("F"), left, len(leaves))

			// A root move onto G writes a record after the free.
			if ripe != "unripe" {
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "g2", Ref: gr}}); err != nil {
					t.Fatal(err)
				}
			}
			if ripe == "ripe" {
				clk.Add(block_gc.DefaultGrace + time.Second)
				nets := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{Now: v.now})
				if _, err := nets.Collect(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if ripe == "unripe behind a record" {
				clk.Add(block_gc.DefaultGrace / 2)
			}
			parent, ref := "F", f
			if tc.who == "a new parent" {
				parent, ref = "P", np
			}
			if err := v.SetBucketRoots(ctx, "b", tr.entries(parent), []block.NamedRoot{{Name: "p", Ref: ref}}); err != nil {
				t.Fatal(err)
			}

			// Collect to quiescence across the grace periods, with the stopped
			// collector resumed or a new one.
			resumed, probe.stopped = true, false
			if tc.restart {
				c = block_gc.NewCollector(collectorStore{v: v}, opts)
			}
			for range 4 {
				clk.Add(block_gc.DefaultGrace/2 + time.Second)
				for more := true; more; {
					res, err := c.Collect(ctx)
					if err != nil {
						t.Fatal(err)
					}
					freed, more = freed+res.Freed, res.More
				}
			}
			audit, err := v.Audit(ctx)
			if err != nil {
				t.Fatal(err)
			}
			lost := 0
			for _, name := range leaves {
				if !stored(name) {
					lost++
				}
			}
			t.Logf("freed %d, holes %d, leaves lost %d of %d, %s stored %t", freed, audit.Holes, lost, len(leaves), parent, stored(parent))
			if lost != 0 || !stored(parent) {
				t.Errorf("%s lists %d freed leaves", parent, lost)
			}
		})
	}

	for _, mid := range []string{"a crash", "children held again", "a second parent's release", "the releasing block put again", "a crash mid put again", "the releasing block lost"} {
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
			// The free deletes F in the transaction that starts its paged
			// release, which then carries F's downs.
			requireStored(t, v, tr, false, "F")

			// Act while the release is in progress, then collect to the end
			// and once more after every record ages.
			kept := []string{"G", first, last}
			var pages [2]uint64
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
			case "the releasing block put again", "a crash mid put again":
				pages[0] = livenessState(t, v).GetUpSeq()
				if err := v.SetBucketRoots(ctx, "b", tr.entries("F"), []block.NamedRoot{{Name: "f", Ref: f}}); err != nil {
					t.Fatal(err)
				}
				pages[1] = livenessState(t, v).GetUpSeq()
				kept = append(slices.Clone(leaves), "F", "G")
			case "the releasing block lost":
				punchHole(t, v, f)
			}
			// The free already deleted F, so the put again is born, and its
			// up pages, which give the leaves back their holder, net across
			// slices; stop with them partly netted.
			if mid == "a crash mid put again" {
				probe := &stopMidRelease{Store: collectorStore{v: v}, v: v, pages: pages}
				c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
				if _, err := c.Collect(ctx); !errors.Is(err, errMidRelease) {
					t.Fatalf("collect = %v, want it stopped with the put again's ups partly netted", err)
				}
			}
			res := collectAll(t, v)
			clk.Add(block_gc.DefaultGrace + time.Second)
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
		// The free already deleted F; its release pages
		// carry its downs.
		requireStored(t, v, tr, false, "F")
		requireStored(t, v, tr, true, h)
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
		// them go in commits one slice nets. D1's free moves E1 first, so
		// D2's list no longer fits the slice and its free waits for the
		// next. The store then loses D2 with its list, so D2 is never
		// freed, its leaves keeping its holder as leaked garbage, and the
		// frees go on: G, let go after, is freed.
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
		// Stop once D1 is freed and D2 still waits.
		stored := func(name string) bool {
			_, found, err := v.GetBlock(ctx, tr[name].Ref)
			if err != nil {
				t.Fatal(err)
			}
			return found
		}
		probe := &stopMidRelease{Store: collectorStore{v: v}, v: v, until: func(*block_gc.LivenessState) bool { return !stored("D1") && stored("D2") }}
		c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
		if _, err := c.Collect(ctx); !errors.Is(err, errMidRelease) {
			t.Fatalf("collect = %v, want it stopped with D2's free waiting", err)
		}

		// Lose D2, let G go, and collect past the grace period.
		punchHole(t, v, d2)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "g", Previous: g}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "D1", "E1", "G")
		requireStored(t, v, tr, true, leaves[0], leaves[len(leaves)-1])
		requireAudit(t, v, uint64(len(leaves)))
	})

	t.Run("format 9 upgrade over a stale queue and a crash mid carry", func(t *testing.T) {
		// The format 9 store of format9Owed, whose upgrade crashed after
		// carrying one count record, opens with frees on. Before the
		// adoption a writer stores N -> C under a new root and releases
		// main, so A and B die while C keeps N. The stale queue entries name
		// the live S and C, the owed releases would move C down twice, and
		// the rerun carry meets a record already carried: none may free a
		// block a root reaches or leave a dead one with an invented holder.
		store := store_kvtx_inmem.NewStore()
		keys := store_kvkey.NewDefaultKVKey()
		tr := format9Owed(t)
		writeFormat9Owed(t, store, keys, tr)
		tx, err := store.NewTransaction(t.Context(), true)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := block_gc.NewRecords(tx, livenessPrefix).CarryFormat9Counts(t.Context(), nil, 1); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
		v, clk := openLivenessVolume(t, store, true)
		n := tr.add(t, "N", "C")
		if err := v.SetBucketRoots(t.Context(), "b", tr.entries("N"), []block.NamedRoot{
			{Name: "n", Ref: n},
			{Name: "main", Previous: tr["A"].Ref},
		}); err != nil {
			t.Fatal(err)
		}
		waitAdopted(t, v)
		for range 3 {
			collectSettled(t, v, clk)
		}
		requireStored(t, v, tr, true, "N", "C", "S")
		requireStored(t, v, tr, false, "A", "B", "P", "G", "H2")
		if holders, _ := blockHolders(t, v, tr, "C"); holders != 1 {
			t.Fatalf("C has %d holders, want N's alone", holders)
		}
		requireAudit(t, v, 0)
	})

	t.Run("root lands on a held child between the netting and the free", func(t *testing.T) {
		// Roots q and z let Q -> X and Z go. Once a pass nets that, and
		// before it frees, a root moves onto X, which Q still holds, so the
		// Land leaves X's count alone and logs an up. Q's free then takes X
		// to zero at the up log's end, which the up cursor has not reached,
		// so with a writer waiting, which frees one block a slice, the
		// slices that follow keep X until the up nets.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		x := tr.add(t, "X")
		q := tr.add(t, "Q", "X")
		z := tr.add(t, "Z")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "Q", "Z"), []block.NamedRoot{{Name: "q", Ref: q}, {Name: "z", Ref: z}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "q", Previous: q}, {Name: "z", Previous: z}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		rooted := false
		root := func(p block_gc.Phase) func() {
			if p == block_gc.PhaseSweep && !rooted {
				rooted = true
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "x", Ref: x}}); err != nil {
					t.Error(err)
				}
			}
			return func() {}
		}
		waiting := func() bool { return true }
		c := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{Frees: true, Now: v.now, Phase: root, WritersWaiting: waiting})
		if res, err := c.Collect(ctx); err != nil || !rooted || res.Freed != 2 {
			t.Fatalf("collect freed %d err %v rooted %v, want Q and Z freed and X kept", res.Freed, err, rooted)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "X")
		requireStored(t, v, tr, false, "Q", "Z")
		requireAudit(t, v, 0)
	})

	t.Run("two born parents name one stored child", func(t *testing.T) {
		// Root c holds C. One Land stores P1 and P2, each naming C, under
		// roots p1 and p2, which gives C two ups, one for each edge. Once
		// root c and root p1 let go and P1 is freed, P2 still holds C.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		cref := tr.add(t, "C")
		p1 := tr.add(t, "P1", "C")
		p2 := tr.add(t, "P2", "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "c", Ref: cref}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P1", "P2"), []block.NamedRoot{{Name: "p1", Ref: p1}, {Name: "p2", Ref: p2}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "c", Previous: cref}, {Name: "p1", Previous: p1}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 1 {
			t.Fatalf("freed %d, want P1 alone", res.Freed)
		}
		requireStored(t, v, tr, true, "P2", "C")
		if holders, _ := blockHolders(t, v, tr, "C"); holders != 1 {
			t.Fatalf("C has %d holders, want P2's", holders)
		}
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
					_, err := v.landFill(ctx, px, "b", tr.entries("P"))
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
			// the up pair of H's second holder is still in the up log at the
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
			if res, err := block_gc.Audit(ctx, probe, clk.Now(), true); err != nil || res.Holes != 1 {
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
		// a root on them. A Volume with frees on refuses the first batch
		// and stores nothing; one that lets the puts in finds both garbage
		// once the grace period passes.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		tr.add(t, "P", "C")
		if _, err := v.PutBlockBatch(ctx, tr.entries("C")); !errors.Is(err, block.ErrUnheldPut) {
			t.Fatalf("unheld put returned %v, want ErrUnheldPut", err)
		}
		requireStored(t, v, tr, false, "C")
		observePuts(v)
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
		// both. C's put alone is unheld in its own batch.
		v, clk := newLivenessVolume(t, true)
		observePuts(v)
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

	t.Run("child put again with its stored parent", func(t *testing.T) {
		// A staged copy puts P under main before its child C, then puts C in
		// a batch with P again. P's list holds C, so a Volume with frees on
		// lets the batch in; C alone it refuses.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		p := tr.add(t, "P", "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		if _, err := v.PutBlockBatch(ctx, tr.entries("C")); !errors.Is(err, block.ErrUnheldPut) {
			t.Fatalf("unheld put returned %v, want ErrUnheldPut", err)
		}
		if _, err := v.PutBlockBatch(ctx, tr.entries("P", "C")); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "C")
		requireAudit(t, v, 0)
	})

	t.Run("parent collected before its child is stored", func(t *testing.T) {
		// A session puts R2 under main before its child C2, logging an up
		// for C2. Main lets R2 go and the collector frees it; the free
		// takes C2's holder although C2 has no list yet. C2 then lands alone
		// and nothing holds it, so it goes after the grace period.
		v, clk := newLivenessVolume(t, true)
		observePuts(v)
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

	// The admit subtests hold a payload tree P -> L that a stage built
	// earlier, as a World's admit does: payloads moves from Q onto a new H
	// naming P and Q, requiring its blocks held.
	t.Run("admit after the tree was freed", func(t *testing.T) {
		// The stage's release nets and frees P and L: refused.
		v, clk, tr, admit := newAdmitVolume(t, true)
		buildAndRelease(t, v, clk, tr)
		requireStored(t, v, tr, false, "P", "L")
		requireNotHeld(t, v, tr, admit())
	})

	t.Run("admit over a Dead parent", func(t *testing.T) {
		// P is stored and Dead, and still holds L, but the admit's root
		// names H over P, which nothing holds: refused.
		v, clk, tr, admit := newAdmitVolume(t, true)
		freeUnderDeadParent(t, v, clk, tr)
		requireNotHeld(t, v, tr, admit())
	})

	t.Run("admit after the collector freed a Dead parent and its child", func(t *testing.T) {
		// A collector that frees takes Dead P, and L with it. The admit over H naming P, and the
		// admit storing P again over L, are both refused, and no root
		// names a freed block.
		v, clk, tr, admit := newAdmitVolume(t, true)
		freeUnderDeadParent(t, v, clk, tr)
		for range 3 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
		}
		requireStored(t, v, tr, false, "P", "L")
		for _, name := range []string{"P", "L"} {
			if listed, held := admitRecords(t, v, tr, name); listed || held {
				t.Fatalf("freed %s listed=%v held=%v, want neither", name, listed, held)
			}
		}
		requireNotHeld(t, v, tr, admit())
		requireNotHeld(t, v, tr, admit("P"))
		if got, err := v.GetBucketRoot(t.Context(), "b", "payloads"); err != nil || !got.EqualsRef(tr["Q"].Ref) {
			t.Fatalf("payloads holds %v after the refusals: %v", got, err)
		}
		requireStored(t, v, tr, true, "Q")
		requireAudit(t, v, 0)
	})

	t.Run("put again of a Dead parent holds its child until it nets", func(t *testing.T) {
		// The release nets with frees off, so P is Dead on the free queue
		// and holds L. A put of P again takes it off the queue and holds it
		// for a grace period: a collect that frees, within it, frees nothing
		// and nets the put's up, so P is held and the admit lands, and the
		// admitted tree stays whole once the put's down nets.
		v, clk, tr, admit := newAdmitVolume(t, true)
		ctx := t.Context()
		stage, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		stagePut(t, v, stage, tr, "L", "P")
		release()
		if err := v.withDirectAtomic(ctx, nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		netAll(t, v)
		if _, err := v.PutBlockBatch(ctx, tr.entries("P")); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		if res := collectAll(t, v); res.Freed != 0 {
			t.Fatalf("freed %d before the put again netted", res.Freed)
		}
		requireStored(t, v, tr, true, "P", "L")
		if err := admit(); err != nil {
			t.Fatalf("admit over P held by its put again = %v, want commit", err)
		}
		requireAdmitKept(t, v, clk, tr)
	})

	t.Run("admit putting a Dead parent again over its freed child", func(t *testing.T) {
		// (j) The admit's batch stores P again, which proves P's bytes, and
		// Dead P still holds L, so the admit lands.
		v, clk, tr, admit := newAdmitVolume(t, true)
		freeUnderDeadParent(t, v, clk, tr)
		requireDeadParentAdmit(t, v, clk, tr, admit("P"))
	})

	t.Run("admit giving birth to a payload root over its freed child", func(t *testing.T) {
		// (k) The tree was freed whole; the admit's batch stores P anew but
		// not L: refused.
		v, clk, tr, admit := newAdmitVolume(t, true)
		buildAndRelease(t, v, clk, tr)
		requireStored(t, v, tr, false, "P", "L")
		requireNotHeld(t, v, tr, admit("P"))
	})

	t.Run("admit repeated with the tree in its batch", func(t *testing.T) {
		// As (j), then the writer repeats the admit storing the whole tree in
		// its batch, as ErrRootNotHeld promises: commits. The repeat moves
		// onto a new H naming P and M over L, a diamond three deep. The
		// first admit lands, since Dead P holds L, and the repeat moves on
		// from it.
		v, clk, tr, admit := newAdmitVolume(t, true)
		freeUnderDeadParent(t, v, clk, tr)
		if err := admit("P"); err != nil {
			t.Fatalf("admit over L held by Dead P = %v, want commit", err)
		}
		prev := tr["H"].Ref
		tr.add(t, "M", "L")
		h := tr.add(t, "H", "P", "M", "Q")
		roots := []block.NamedRoot{{Name: "payloads", Ref: h, Previous: prev, RequireHeld: true}}
		if err := v.SetBucketRoots(t.Context(), "b", tr.entries("H", "M", "P", "L"), roots); err != nil {
			t.Fatal(err)
		}
		requireAdmitKept(t, v, clk, tr)
		requireStored(t, v, tr, true, "M")
	})

	for _, counted := range []bool{false, true} {
		name := "admit observed past a failed read"
		if counted {
			name += " after a count"
		}
		t.Run(name, func(t *testing.T) {
			// The check's stage read fails on P, Dead over its child
			// L. Enforcing returns the error; observing counts nothing and
			// lands the move as with the check off. With counted, the first
			// stage read answers false, so P is counted as H's child, and a
			// second strict root on P then fails its read: the count made
			// before the failure is dropped too.
			v, clk, tr, _ := newAdmitVolume(t, true)
			freeUnderDeadParent(t, v, clk, tr)
			errRead := errors.New("stage read failed")
			roots := []block.NamedRoot{{Name: "payloads", Ref: tr["H"].Ref, Previous: tr["Q"].Ref, RequireHeld: true}}
			if counted {
				roots = append(roots, block.NamedRoot{Name: "again", Ref: tr["P"].Ref, RequireHeld: true})
			}
			land := func(mode block_gc.HeldMode) error {
				reads := 0
				return v.withDirectAtomic(t.Context(), func(p *physicalTx) error {
					_, err := v.landMoves(t.Context(), p, tr.entries("H"), block_gc.BucketRoots{
						BucketID: "b",
						Roots:    roots,
						Mode:     mode,
						StageHeld: func(context.Context, *block.BlockRef) (bool, error) {
							reads++
							if counted && reads == 1 {
								return false, nil
							}
							return false, errRead
						},
						Observed: v.observeNotHeld,
					})
					return err
				})
			}
			// Enforcing refuses at the first block not held, before the
			// failing read when counted.
			want := errRead
			if counted {
				want = block.ErrRootNotHeld
			}
			if err := land(block_gc.HeldEnforce); !errors.Is(err, want) {
				t.Fatalf("enforced Land = %v, want %v", err, want)
			}
			if err := land(block_gc.HeldObserve); err != nil {
				t.Fatalf("observed Land = %v, want commit", err)
			}
			if got, err := v.GetBucketRoot(t.Context(), "b", "payloads"); err != nil || !got.EqualsRef(tr["H"].Ref) {
				t.Fatalf("payloads holds %v after the observed Land: %v", got, err)
			}
			h, _ := v.GetHealthSnapshotWithWait()
			if n := h.GetNotHeldRootBlocks() + h.GetNotHeldChildBlocks(); n != 0 {
				t.Fatalf("a failed check counted %d", n)
			}
		})
	}

	for _, freed := range []string{"put again", "born"} {
		t.Run("admit storing P "+freed+" over a child a live stage holds", func(t *testing.T) {
			// (l) As (j) and (k), with an open stage holding L: commits, and
			// a collector that frees keeps the whole tree.
			v, clk, tr, admit := newAdmitVolume(t, true)
			if freed == "born" {
				buildAndRelease(t, v, clk, tr)
			} else {
				freeUnderDeadParent(t, v, clk, tr)
			}
			stage, release, err := v.OpenStage(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			stagePut(t, v, stage, tr, "L")
			if err := admit("P"); err != nil {
				t.Fatal(err)
			}
			release()
			requireAdmitKept(t, v, clk, tr)
		})
	}

	t.Run("admit observed with frees off", func(t *testing.T) {
		// The default with frees off observes: the move lands over the
		// released tree and counts P, a child outside the batch.
		v, clk := newLivenessVolume(t, false)
		if v.heldMode != block_gc.HeldObserve {
			t.Fatalf("mode %v with frees off, want observe", v.heldMode)
		}
		tr, admit := admitTree(t, v)
		buildAndRelease(t, v, clk, tr)
		if err := admit(); err != nil {
			t.Fatal(err)
		}
		h, _ := v.GetHealthSnapshotWithWait()
		root, child := h.GetNotHeldRootBlocks(), h.GetNotHeldChildBlocks()
		if root != 0 || child != 1 {
			t.Fatalf("observed root=%d child=%d, want 0 and 1", root, child)
		}
	})

	t.Run("admit with the check off", func(t *testing.T) {
		// Off with frees on checks nothing: the move lands and counts none.
		v, clk := newLivenessVolume(t, true)
		v.collector = v.newCollector(collectorStore{v: v}, false, block_gc.DefaultGrace)
		v.heldMode = rootHeldMode(store_kvtx.RootHeldCheck_ROOT_HELD_CHECK_OFF, true)
		tr, admit := admitTree(t, v)
		buildAndRelease(t, v, clk, tr)
		if err := admit(); err != nil {
			t.Fatal(err)
		}
		h, _ := v.GetHealthSnapshotWithWait()
		if n := h.GetNotHeldRootBlocks() + h.GetNotHeldChildBlocks(); n != 0 {
			t.Fatalf("observed %d with the check off", n)
		}
	})

	t.Run("admit over bytes without a list", func(t *testing.T) {
		v, _, tr, admit := newAdmitVolume(t, true)
		ref, _, err := v.Store.PutBlock(t.Context(), tr["P"].Data, &block.PutOpts{RefsKnown: true})
		if err != nil || !ref.EqualsRef(tr["P"].Ref) {
			t.Fatalf("stored P without a list as %v: %v", ref, err)
		}
		requireNotHeld(t, v, tr, admit())
	})

	t.Run("admit after a flush cut after its first slice", func(t *testing.T) {
		// The open stage stored L only; P is missing: refused.
		v, _, tr, admit := newAdmitVolume(t, true)
		stage, release, err := v.OpenStage(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		stagePut(t, v, stage, tr, "L")
		requireNotHeld(t, v, tr, admit())
	})

	t.Run("admit retried over a rebuilt tree", func(t *testing.T) {
		// The release nets P and L to zero with frees off. A second stage
		// puts the same tree again, and the admit commits at once, before
		// netting gives P a holder. Once that stage is released and its
		// release nets, a collector that frees keeps the whole tree.
		v, clk, tr, admit := newAdmitVolume(t, false)
		buildAndRelease(t, v, clk, tr)
		stage, release, err := v.OpenStage(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		stagePut(t, v, stage, tr, "L", "P")
		if err := admit(); err != nil {
			t.Fatal(err)
		}
		requireStored(t, v, tr, true, "H", "P", "L", "Q")
		release()
		requireAdmitKept(t, v, clk, tr)
	})

	t.Run("admit under a live hold", func(t *testing.T) {
		v, clk, tr, admit := newAdmitVolume(t, true)
		if err := v.SetBucketRoots(t.Context(), "b", tr.entries("L", "P"), []block.NamedRoot{{Name: "other", Ref: tr["P"].Ref}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		if err := admit(); err != nil {
			t.Fatal(err)
		}
		requireStored(t, v, tr, true, "H", "P", "L", "Q")
	})

	t.Run("admit after the stage was released", func(t *testing.T) {
		// The stage's release nets P and L to zero with frees off, so both
		// are stored but nothing holds them: refused. The release nets first
		// because until it does, P still reads one holder and the admit
		// commits; that is safe, since the admit's up pair and the release's
		// down pair net together and leave P held.
		v, clk, tr, admit := newAdmitVolume(t, false)
		buildAndRelease(t, v, clk, tr)
		requireStored(t, v, tr, true, "P", "L")
		requireNotHeld(t, v, tr, admit())
	})

	t.Run("admit before the stage's release nets", func(t *testing.T) {
		// The admit commits while the release is in the logs and not
		// netted, so P still reads its implied holder. The release then nets
		// alone, older than Grace while the admit's record is not, which
		// brings P and L to zero; their ripe sequence lies past the admit's
		// record, so no free runs until the admit nets and holds them again.
		v, clk, tr, admit := newAdmitVolume(t, true)
		ctx := t.Context()
		stage, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		stagePut(t, v, stage, tr, "L", "P")
		release()
		if err := v.withDirectAtomic(ctx, nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace / 2)
		if err := admit(); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace/2 + time.Second)
		if res := collectAll(t, v); res.Freed != 0 {
			t.Fatalf("freed %d between the release and the admit", res.Freed)
		}
		requireStored(t, v, tr, true, "H", "P", "L", "Q")
		for range 2 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
		}
		requireStored(t, v, tr, true, "H", "P", "L", "Q")
		requireAudit(t, v, 0)
	})

	t.Run("admit evidenced by another holder's stage", func(t *testing.T) {
		// After the writer's release nets to zero, another holder's stage
		// puts the tree again while the writer's own stage holds nothing of
		// it. The admit commits on that evidence, the other stage is released
		// right after, and a collector that frees keeps the whole tree.
		v, clk, tr, admit := newAdmitVolume(t, false)
		ctx := t.Context()
		buildAndRelease(t, v, clk, tr)
		_, own, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer own()
		other, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		stagePut(t, v, other, tr, "L", "P")
		if err := admit(); err != nil {
			t.Fatal(err)
		}
		release()
		requireAdmitKept(t, v, clk, tr)
	})

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

	t.Run("unadopted", func(t *testing.T) {
		// A Volume that predates adoption logs and frees nothing.
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
		if _, err := block_gc.Adopt(probeCtx, probe, v.now); !errors.Is(err, context.Canceled) || len(probe.at) != 0 {
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
		if _, err := block_gc.Adopt(ctx, probe, v.now); err != nil || len(probe.at) != 0 {
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
		// of X or Y and queues both, so the generation alone keeps them
		// under the root: while it is Open the free parks every block it
		// takes, the young N included. The walk then runs on the adopted
		// Volume: it refuses a decoded list that drops P's child, and
		// rewrites R's list, which records that the counts are short of R's
		// holders of X and Y. The flip finds that record and returns the
		// Volume to unadopted; the recount deletes the parked and queued
		// records, and the settle frees only N.
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
		if res := collectAll(t, v); res.Parked != 2 || res.Freed != 0 || res.Flipped {
			t.Fatalf("collect while Open = %+v, want X and N parked and no flip", res)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "y", Previous: y}}); err != nil {
			t.Fatal(err)
		}
		requireStored(t, v, tr, true, "R", "P", "Z", "X", "Y", "N")
		kind := func(k byte) []byte { return append(slices.Clone(livenessPrefix), k) }

		// A walk that decodes X and a P that lost its child refuses P,
		// keeps P's stored list, and writes nothing that calls for a
		// recount. The hold of Y left Y on the front.
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
		if res.Dropped != 1 || !res.DroppedSample[0].EqualsRef(p) || res.Verified != 1 || res.Front == 0 {
			t.Fatalf("rebuild with P dropping Z = %+v, want P refused and the front standing", res)
		}
		if got := livenessRecords(t, store, 'c')[string(kind('c'))+string(block_gc.BlockKey(p))]; got != string(block_gc.EncodeChildren(tr["P"].Refs)) {
			t.Fatal("refused rebuild changed P's stored list")
		}
		if n := countPrefix(t, store, kind('Y')); n != 0 {
			t.Fatalf("rebuild that rewrote nothing left %d recount records", n)
		}

		// A rerun that decodes R, P and the leaves rewrites R's list, which
		// records the recount the counts now owe.
		if res := rebuild(tr["X"], tr["R"], tr["P"], tr["Z"], tr["Y"]); res.Rewritten != 1 || res.Dropped != 0 {
			t.Fatalf("rerun rebuild = %+v, want R rewritten", res)
		}
		if n := countPrefix(t, store, kind('Y')); n != 1 {
			t.Fatalf("rewrite on an adopted Volume left %d recount records, want 1", n)
		}

		// The flip unadopts rather than flip; the recount counts R's holders
		// of X and Y and deletes every record from before, and the settle
		// then frees N alone.
		total := settleAll(t, v, clk, store, tr)
		if !total.Unadopted {
			t.Fatalf("settle = %+v, want the rewrite to unadopt the Volume", total)
		}
		for _, k := range []byte{'X', 'Y'} {
			if n := countPrefix(t, store, kind(k)); n != 0 {
				t.Fatalf("settle left %d records of kind %c", n, k)
			}
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "P", "Z", "X", "Y")
		requireStored(t, v, tr, false, "N")
		requireAudit(t, v, 0)

		// A rebuild of a Volume whose old generation settled is refused, since
		// a rewrite then would leave the counts short with no flip to come.
		if _, err := v.OpenListRebuild(ctx); !errors.Is(err, block_gc.ErrListsVerified) {
			t.Fatalf("rebuild of a settled Volume err %v, want ErrListsVerified", err)
		}
	})

	t.Run("reput of an older block stored without a list", func(t *testing.T) {
		// A format 6 store holds Q under main, naming P, but a crash left Q's
		// list without P and P stored without a list of its own, so the
		// adoption passes and counts nothing for P. A put of P again under
		// the new N writes P's list, and Land keeps one holder more for P,
		// since its bytes were stored. P's bytes predate the put, so P is
		// older: once a later adoption recounts the lists and finds P at
		// zero, the free parks it, and Q still reaches it.
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

		// Put P again under N, then release N: while the generation is Open
		// the free parks N, and keeps P at the holder the reput kept.
		n := tr.add(t, "N", "P")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P", "N"), []block.NamedRoot{{Name: "new", Ref: n}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "new", Previous: n}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 0 || res.Parked != 1 {
			t.Fatalf("collect after the reput = %+v, want N parked and P kept", res)
		}
		requireStored(t, v, tr, true, "Q", "P", "N")

		// A recount finds N at zero, and the free parks it; P keeps N's
		// holder.
		markUnadopted(t, v)
		if res, err := v.Adopt(ctx); err != nil || res.Queued != 1 {
			t.Fatalf("readopt = %+v err %v, want N queued", res, err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Parked == 0 || res.Freed != 0 {
			t.Fatalf("collect after the recount = %+v, want N parked and nothing freed", res)
		}
		requireStored(t, v, tr, true, "Q", "P", "N")

		// The settle frees N, whose release leaves P at zero, and the walk
		// from main decodes Q and gives P its holder back before the flip,
		// so P stays.
		settleAll(t, v, clk, store, tr)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "Q", "P")
		requireStored(t, v, tr, false, "N")
		requireAudit(t, v, 0)
	})

	t.Run("parked block requeued once the generation flips", func(t *testing.T) {
		// A format 6 store holds R -> X under main and an unreached old block
		// G that also names X. Adopted with the generation Open, the free
		// parks G. Once the walk verifies R and X the generation flips, and
		// the parked G comes back as unmarked. The requeued free leaves it
		// to the sweep, which releases G's holder on X once, so R's holder
		// keeps X.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := store_kvtx_inmem.NewStore()
		tr := testTree{}
		tr.add(t, "X")
		tr.add(t, "R", "X")
		tr.add(t, "G", "X")
		writeLegacyStore(t, store, keys, tr, "R", true, "X", "R", "G")
		v, clk := openUnadoptedVolume(t, store)
		if res, err := v.Adopt(ctx); err != nil || res.Queued != 1 {
			t.Fatalf("adopt = %+v err %v, want G queued", res, err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Parked != 1 || res.Freed != 0 {
			t.Fatalf("collect while Open = %+v, want G parked", res)
		}
		requireStored(t, v, tr, true, "R", "X", "G")
		rb, err := v.OpenListRebuild(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"R", "X"} {
			if err := rb.Verify(ctx, tr[name].Ref, tr[name].Refs); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := rb.Finish(ctx); err != nil {
			t.Fatal(err)
		}
		if res := settleAll(t, v, clk, store, tr); res.Swept != 1 || res.Freed != 0 {
			t.Fatalf("settle = %+v, want G swept and nothing freed", res)
		}
		requireStored(t, v, tr, true, "R", "X")
		requireStored(t, v, tr, false, "G")
		requireAudit(t, v, 0)
	})

	t.Run("parked block a commit names before the flip", func(t *testing.T) {
		// A format 6 store holds the root A and an unreached block C -> F
		// whose list lacks F. The adoption counts no holder of C or F, and
		// the free parks both. A commit naming F from a new block C2 is
		// refused until it puts F again, and the walk verifies A and F with
		// C's list still cut, since no root reaches C. The settle deletes C
		// alone.
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
			t.Fatalf("collect while Open = %+v, want C and F parked", res)
		}

		// C2 -> F alone is refused while F is parked at zero; with F, the
		// put gives birth to F again under C2.
		c2 := tr.add(t, "C2", "F")
		c2root := []block.NamedRoot{{Name: "c2", Ref: c2}}
		var parked *block_gc.AbsentError
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C2"), c2root); !errors.As(err, &parked) || !parked.Ref.EqualsRef(tr["F"].Ref) {
			t.Fatalf("put of C2 alone err %v, want the parked F refused", err)
		}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C2", "F"), c2root); err != nil {
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
		if res, err := rb.Finish(ctx); err != nil || res.Rewritten != 0 {
			t.Fatalf("rebuild = %+v err %v, want nothing rewritten", res, err)
		}
		settleAll(t, v, clk, store, tr)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "A", "C2", "F")
		requireStored(t, v, tr, false, "C")
		requireAudit(t, v, 0)
	})

	t.Run("old list a put rewrites", func(t *testing.T) {
		// A format 6 store holds the root A and unreached blocks C -> F,
		// whose list lacks F, G and H. The walk verifies A and F. A put of
		// G, whose list is whole, writes nothing that calls for a recount. A
		// put of C under a root rewrites C's list to name F and records the
		// recount; the flip then returns the Volume to unadopted, and once
		// the recount runs, the settle deletes H, which no put revived, and
		// keeps F under C.
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
		if _, err := rb.Finish(ctx); err != nil {
			t.Fatal(err)
		}
		rewritten := slices.Concat(livenessPrefix, []byte{'Y'})

		// G's list is whole: the put leaves no recount record.
		g := tr["G"].Ref
		if err := v.SetBucketRoots(ctx, "b", tr.entries("G"), []block.NamedRoot{{Name: "g", Ref: g}}); err != nil {
			t.Fatal(err)
		}
		if a := livenessAdoption(t, v); a != block_gc.Adoption_ADOPTION_ADOPTED || countPrefix(t, store, rewritten) != 0 {
			t.Fatalf("after the put of G the Volume is %v, want adopted and no recount", a)
		}

		// C's list lacks F: the put rewrites it and records the recount.
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "c", Ref: c}}); err != nil {
			t.Fatal(err)
		}
		if a := livenessAdoption(t, v); a != block_gc.Adoption_ADOPTION_ADOPTED || countPrefix(t, store, rewritten) != 1 {
			t.Fatalf("after the put of C the Volume is %v, want adopted and a recount recorded", a)
		}
		if found, err := v.GetBlockExists(ctx, tr["H"].Ref); err != nil || !found {
			t.Fatalf("exists H while Open = %v err %v, want found", found, err)
		}
		if res := settleAll(t, v, clk, store, tr); !res.Unadopted {
			t.Fatalf("settle = %+v, want the rewrite to unadopt the Volume", res)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "A", "C", "F", "G")
		requireStored(t, v, tr, false, "H")
		if found, err := v.GetBlockExists(ctx, tr["H"].Ref); err != nil || found {
			t.Fatalf("exists H after the settle = %v err %v, want absent", found, err)
		}
		requireAudit(t, v, 0)
	})

	t.Run("rewriting put supersedes an overlapping adoption", func(t *testing.T) {
		// The store of "old list a put rewrites", unadopted after the
		// format 7 upgrade, with a walk that verified A and F. Adoption A
		// marks the Volume adopting, then B claims it and counts the lists,
		// C's still lacking F. Before B settles, a put of C rewrites C's
		// list. A is superseded by B's claim; B settles, and the recount
		// record the put left makes the flip unadopt and recount. The settle
		// then takes G, which nothing names, and F keeps its holder.
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
			adoptCountRoots: func() { _, errB = block_gc.Adopt(ctx, probeB, v.now) },
		}}
		_, errA := block_gc.Adopt(ctx, probeA, v.now)
		if !errors.Is(errA, block_gc.ErrAdoptSuperseded) || errB != nil {
			t.Fatalf("adoptions = %v and %v with %d and %d hooks left, want A superseded and B adopted", errA, errB, len(probeA.at), len(probeB.at))
		}
		if n := countPrefix(t, store, slices.Concat(livenessPrefix, []byte{'Y'})); n != 1 {
			t.Fatalf("the put of C left %d recount records, want 1", n)
		}
		verify := func(names ...string) {
			rb, err := v.OpenListRebuild(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range names {
				if err := rb.Verify(ctx, tr[name].Ref, tr[name].Refs); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := rb.Finish(ctx); err != nil {
				t.Fatal(err)
			}
		}
		verify("A", "F")
		if res := settleAll(t, v, clk, store, tr); !res.Unadopted {
			t.Fatalf("settle = %+v, want the rewrite to unadopt the Volume", res)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
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
			_, _ = block_gc.Adopt(bCtx, probeB, v.now)
		}()

		// A starts B before its mark and lets B clear before it settles.
		probeA := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptMark: func() { <-claimed },
			adoptSettle: func() {
				releaseB()
				<-cleared
			},
		}}
		_, err := block_gc.Adopt(ctx, probeA, v.now)
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
				if _, err := block_gc.Adopt(ctx, collectorStore{v: v}, v.now); err != nil {
					t.Error(err)
				}
			},
		}}
		if _, err := block_gc.Adopt(ctx, probe, v.now); !errors.Is(err, block_gc.ErrAdoptSuperseded) || len(probe.at) != 0 {
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

// The transactions of block_gc.Adopt, in order, on a Volume whose records
// each fit in one slice: the claim, the pop mark's raise, the take-out of
// parked blocks from the run, the clears of counts, the queue, parked
// blocks, the up and down logs, born marks, moved-root marks, release pages,
// recount records and unmarked counts, then the mark, the root count, the
// child list count and the settle.
const (
	// adoptClearCounts clears the count records an earlier run left.
	adoptClearCounts = 4
	// adoptMark marks the Volume adopting.
	adoptMark = 14
	// adoptCountRoots counts the roots.
	adoptCountRoots = 15
	// adoptCountChildren counts the child lists' holders.
	adoptCountChildren = 16
	// adoptSettle settles the stored blocks' counts.
	adoptSettle = 17
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

// waitAdopted waits for the background adoption of v to mark it adopted,
// and returns the state it left.
func waitAdopted(t *testing.T, v *Volume) *block_gc.LivenessState {
	t.Helper()
	if v.adoptDone == nil {
		t.Fatal("the Volume runs no background adoption")
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(time.Millisecond) {
		state := livenessState(t, v)
		if state.GetAdoption() == block_gc.Adoption_ADOPTION_ADOPTED {
			return state
		}
		if time.Now().After(deadline) {
			t.Fatalf("background adoption left the Volume %v", state.GetAdoption())
		}
	}
}

// TestLivenessSettle walks the sequences in which a block a holder reaches
// names a child whose list a format 6 store cut short, so no holder counts
// the block below that list: a put again and a fill while the unmarked run
// builds, fills top down and under a held block, and a refilled hole. In each
// the settle keeps the block below while a root reaches it. It then checks
// each barrier a Land keeps and each clause the flip waits on, and that the
// sweep deletes exactly the old blocks nothing reaches.
func TestLivenessSettle(t *testing.T) {
	// adoptOldStore writes a format 6 store into store holding the root A
	// and, for each name in cut, the block's bytes with an empty child list,
	// as a crash left them, then adopts it.
	adoptOldStore := func(t *testing.T, store kvtx.Store, tr testTree, root string, listed []string, cut ...string) (*Volume, *testClock) {
		t.Helper()
		ctx := t.Context()
		writeLegacyStore(t, store, store_kvkey.NewDefaultKVKey(), tr, root, true, listed...)
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range cut {
			key := slices.Concat(livenessPrefix, []byte{'c'}, block_gc.BlockKey(tr[name].Ref))
			if err := tx.Set(ctx, key, block_gc.EncodeChildren(nil)); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v, clk := openUnadoptedVolume(t, store)
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		return v, clk
	}
	// oldStoreOn is adoptOldStore, checking the front when the test ends.
	oldStoreOn := func(t *testing.T, store kvtx.Store, tr testTree, root string, listed []string, cut ...string) (*Volume, *testClock) {
		t.Helper()
		v, clk := adoptOldStore(t, store, tr, root, listed, cut...)
		t.Cleanup(func() { requireFront(t, v) })
		return v, clk
	}
	// oldStore is oldStoreOn over a new in-memory store.
	oldStore := func(t *testing.T, tr testTree, root string, listed []string, cut ...string) (*Volume, *testClock, kvtx.Store) {
		t.Helper()
		store := store_kvtx_inmem.NewStore()
		v, clk := oldStoreOn(t, store, tr, root, listed, cut...)
		return v, clk, store
	}
	run := slices.Concat(livenessPrefix, []byte{'I'})
	front := slices.Concat(livenessPrefix, []byte{'G'})
	has := func(t *testing.T, store kvtx.Store, prefix []byte, ref *block.BlockRef) bool {
		t.Helper()
		tx, err := store.NewTransaction(t.Context(), false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		var key []byte
		if ref != nil {
			key = block_gc.BlockKey(ref)
		}
		found, err := tx.Exists(t.Context(), slices.Concat(prefix, key))
		if err != nil {
			t.Fatal(err)
		}
		return found
	}
	fill := func(t *testing.T, v *Volume, tr testTree, names ...string) {
		t.Helper()
		err := v.withDirectFill(t.Context(), func(p *physicalTx) error {
			_, err := v.landFill(t.Context(), p, "b", tr.entries(names...))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	rootOnto := func(t *testing.T, v *Volume, tr testTree, name string, put ...string) error {
		t.Helper()
		return v.SetBucketRoots(t.Context(), "b", tr.entries(put...), []block.NamedRoot{{Name: "main", Ref: tr[name].Ref}})
	}
	verify := func(t *testing.T, v *Volume, tr testTree, names ...string) block_gc.ListRebuildResult {
		t.Helper()
		rb, err := v.OpenListRebuild(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			if err := rb.Verify(t.Context(), tr[name].Ref, tr[name].Refs); err != nil {
				t.Fatal(err)
			}
		}
		res, err := rb.Finish(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	// requireKept settles the old generation and checks that below, which
	// main reaches through path, is still stored.
	requireKept := func(t *testing.T, v *Volume, clk *testClock, store kvtx.Store, tr testTree, below, path string) {
		t.Helper()
		res := settleAll(t, v, clk, store, tr)
		_, found, err := v.GetBlock(t.Context(), tr[below].Ref)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("main reaches %s through %s, and the settle deleted it (%+v)", below, path, res)
		}
		requireAudit(t, v, 0)
	}
	// collectUntil collects, letting the grace period pass before each pass
	// and walking nothing, until done reports the pass's result as the one
	// wanted.
	collectUntil := func(t *testing.T, v *Volume, clk *testClock, done func(block_gc.CollectResult) bool) block_gc.CollectResult {
		t.Helper()
		for range 16 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			if res := collectAll(t, v); done(res) {
				return res
			}
		}
		t.Fatalf("collection never reached the state wanted; the generation is %v", livenessGeneration(t, v))
		return block_gc.CollectResult{}
	}
	flipped := func(res block_gc.CollectResult) bool { return res.Flipped }
	settled := func(res block_gc.CollectResult) bool { return res.Settled }
	// pairs builds, for each i < n, D<i>, C<i> naming D<i> and B<i> naming
	// C<i>, and returns the names to list and the C names to cut.
	pairs := func(t *testing.T, tr testTree, n int) (listed, cut []string) {
		t.Helper()
		tr.add(t, "A")
		listed = []string{"A"}
		for i := range n {
			s := strconv.Itoa(i)
			tr.add(t, "D"+s)
			tr.add(t, "C"+s, "D"+s)
			tr.add(t, "B"+s, "C"+s)
			listed = append(listed, "D"+s, "C"+s, "B"+s)
			cut = append(cut, "C"+s)
		}
		return listed, cut
	}
	// buildRun runs one collection pass through probe, which calls during
	// once, between two slices of the run's build.
	buildRun := func(t *testing.T, v *Volume, store kvtx.Store, during func()) {
		t.Helper()
		var ran bool
		probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
			if ran || countPrefix(t, store, run) == 0 || has(t, store, run, nil) {
				return
			}
			ran = true
			during()
		}}
		c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
		for {
			res, err := c.Collect(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if !res.More {
				break
			}
		}
		if !ran {
			t.Fatal("the run built in one slice")
		}
	}

	t.Run("frees off and on while the generation is open", func(t *testing.T) {
		// B -> C -> D, with C's list cut, none reached, and one frees-on
		// pass partway through the settle. Frees go off, and a writer roots
		// main onto a new N naming one C. Frees come back on, which recounts.
		// Land keeps the settle's barriers by the stored generation, not by
		// the frees switch, so the walk still finds D below C, and the
		// settle keeps N, C and D.
		tr := testTree{}
		listed, cut := pairs(t, tr, 150)
		store := store_kvtx_inmem.NewStore()
		v1, _ := adoptOldStore(t, store, tr, "A", listed, cut...)
		verify(t, v1, tr, "A")
		if gen := livenessGeneration(t, v1); gen != block_gc.Generation_GENERATION_OPEN {
			t.Fatalf("the adoption left the generation %v, want it open", gen)
		}
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}

		// Frees off: the writer lands N, and the passes free nothing.
		v2, clk2 := openLivenessVolume(t, store, false)
		tr.add(t, "N", "C7")
		if err := rootOnto(t, v2, tr, "N", "N"); err != nil {
			t.Fatal(err)
		}
		for range 3 {
			clk2.Add(block_gc.DefaultGrace + time.Second)
			if res := collectAll(t, v2); res.Freed != 0 || res.Swept != 0 {
				t.Fatalf("a pass with frees off freed %d and swept %d", res.Freed, res.Swept)
			}
		}
		requireFront(t, v2)
		if err := v2.Close(); err != nil {
			t.Fatal(err)
		}

		// Frees on again: the recount, then the settle.
		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		settleAll(t, v, clk, store, tr)
		requireReads(t, v, tr, "N", "C7", "D7")
		requireFront(t, v)
		requireAudit(t, v, 0)
	})

	t.Run("reader sight across an adoption while the generation is open", func(t *testing.T) {
		// R1 -> E in an old store, its generation Open, R1 its world root.
		// A reader sees R1, the root moves to R2, and the Volume adopts
		// again before the reader pins R1: the adoption drops the record of
		// the move, so no seed of the settle walk reaches R1. Passes within
		// the grace period walk, and the flip, which would unmark R1 for
		// the sweep, waits for the adoption's marker; the reader pins R1
		// and the settle keeps it.
		tr := testTree{}
		tr.add(t, "E")
		tr.add(t, "R1", "E")
		v, clk, store := oldStore(t, tr, "R1", []string{"E", "R1"})
		tr.add(t, "R2")
		world := []block.NamedRoot{{Name: "world", Ref: tr["R2"].Ref, Previous: tr["R1"].Ref}}
		if err := v.SetBucketRoots(t.Context(), "space", tr.entries("R2"), world); err != nil {
			t.Fatal(err)
		}
		markUnadopted(t, v)
		if _, err := v.Adopt(t.Context()); err != nil {
			t.Fatal(err)
		}
		for range 4 {
			walkFront(t, v, store, tr)
			collectAll(t, v)
		}
		requireStored(t, v, tr, true, "R1", "E", "R2")
		unpin, err := v.PinBucketRoot(t.Context(), tr["R1"].Ref)
		if err != nil {
			t.Fatal(err)
		}
		settleAll(t, v, clk, store, tr)
		requireStored(t, v, tr, true, "R1", "E", "R2")

		// Once the pin is released, the old tree goes after the grace
		// period.
		unpin()
		if err := v.withDirectAtomic(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
		for range 4 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
		}
		requireStored(t, v, tr, false, "R1", "E")
		requireStored(t, v, tr, true, "R2")
		requireAudit(t, v, 0)
	})

	t.Run("put again while the run builds", func(t *testing.T) {
		// B -> C -> D, with C's list cut, none reached. Between two slices
		// of the run's build, a session puts again a B the build entered,
		// whose C it has not, and moves main onto it. The put marks B's list
		// and pushes C onto the front, which holds the flip until the walk
		// decodes C and finds D.
		tr := testTree{}
		listed, cut := pairs(t, tr, 150)
		v, clk, store := oldStore(t, tr, "A", listed, cut...)
		verify(t, v, tr, "A")
		var pick string
		buildRun(t, v, store, func() {
			for i := range 150 {
				s := strconv.Itoa(i)
				if has(t, store, run, tr["B"+s].Ref) && !has(t, store, run, tr["C"+s].Ref) {
					pick = s
					break
				}
			}
			if pick == "" {
				t.Fatal("no B entered with its C ahead")
			}
			if err := rootOnto(t, v, tr, "B"+pick, "B"+pick); err != nil {
				t.Fatal(err)
			}
		})
		if has(t, store, run, tr["B"+pick].Ref) || !has(t, store, front, tr["C"+pick].Ref) {
			t.Fatal("the put left B in the run or C off the front")
		}
		if gen := livenessGeneration(t, v); gen != block_gc.Generation_GENERATION_OPEN {
			t.Fatalf("generation %v with C on the front, want Open", gen)
		}
		requireKept(t, v, clk, store, tr, "D"+pick, "B"+pick+" and C"+pick+"'s short list")
	})

	t.Run("fill while the run builds", func(t *testing.T) {
		// Between two slices of the run's build, a read fills X naming a C
		// the build has not entered, with X's key behind the build's
		// position, and main moves onto X. The fill pushes C onto the front.
		tr := testTree{}
		listed, cut := pairs(t, tr, 150)
		v, clk, store := oldStore(t, tr, "A", listed, cut...)
		verify(t, v, tr, "A")
		var x, c string
		buildRun(t, v, store, func() {
			var behind []byte
			tx, err := store.NewTransaction(t.Context(), false)
			if err != nil {
				t.Fatal(err)
			}
			err = tx.ScanPrefixKeys(t.Context(), run, func(key []byte) error {
				if k := key[len(run):]; bytes.Compare(k, behind) > 0 {
					behind = bytes.Clone(k)
				}
				return nil
			})
			tx.Discard()
			if err != nil {
				t.Fatal(err)
			}
			for i := range 150 {
				if s := strconv.Itoa(i); !has(t, store, run, tr["C"+s].Ref) {
					c = "C" + s
					break
				}
			}
			for j := 0; c != "" && x == ""; j++ {
				name := "X" + strconv.Itoa(j)
				if bytes.Compare(block_gc.BlockKey(tr.add(t, name, c)), behind) < 0 {
					x = name
				}
			}
			if x == "" {
				t.Fatal("no C ahead of the build")
			}
			fill(t, v, tr, x)
			if err := rootOnto(t, v, tr, x); err != nil {
				t.Fatal(err)
			}
		})
		if !has(t, store, front, tr[c].Ref) {
			t.Fatalf("the fill of %s left %s off the front", x, c)
		}
		requireKept(t, v, clk, store, tr, "D"+c[1:], x+" and "+c+"'s short list")
	})

	// cutC builds A and C -> F with C's list cut, adopts, and verifies A.
	cutC := func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree) {
		t.Helper()
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "F")
		tr.add(t, "C", "F")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "F", "C"}, "C")
		verify(t, v, tr, "A")
		return v, clk, store, tr
	}

	t.Run("fill top down", func(t *testing.T) {
		// A read fills P while its child X is absent, then fills X, which
		// names the old C. Main then moves onto P.
		v, clk, store, tr := cutC(t)
		tr.add(t, "X", "C")
		tr.add(t, "P", "X")
		fill(t, v, tr, "P")
		fill(t, v, tr, "X")
		if err := rootOnto(t, v, tr, "P"); err != nil {
			t.Fatal(err)
		}
		requireKept(t, v, clk, store, tr, "F", "P, X and C's short list")
	})

	t.Run("fill under a held block", func(t *testing.T) {
		// Main holds B, whose child X is absent; a read then fills X, which
		// names the old C.
		v, clk, store, tr := cutC(t)
		tr.add(t, "X", "C")
		tr.add(t, "B", "X")
		if err := rootOnto(t, v, tr, "B", "B"); err != nil {
			t.Fatal(err)
		}
		fill(t, v, tr, "X")
		requireKept(t, v, clk, store, tr, "F", "B, X and C's short list")
	})

	t.Run("refilled hole", func(t *testing.T) {
		// Main holds A -> H -> C -> D, with C's list cut. The store loses H
		// and an audit records the hole, so the walk from A pushes H, which
		// no read resolves, and C stays unreached. H's front entry holds the
		// flip until a put refills H, which pushes C.
		tr := testTree{}
		tr.add(t, "D")
		tr.add(t, "C", "D")
		h := tr.add(t, "H", "C")
		tr.add(t, "A", "H")
		v, clk, store := oldStore(t, tr, "A", []string{"D", "C", "H", "A"}, "C")
		punchHole(t, v, h)
		if res, err := v.Audit(t.Context()); err != nil || res.Holes != 1 {
			t.Fatalf("audit recorded %d holes err %v, want 1", res.Holes, err)
		}
		verify(t, v, tr, "A")
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Flipped || countPrefix(t, store, front) != 1 || !has(t, store, front, h) {
			t.Fatalf("collect with a hole = %+v, want no flip and only H on the front", res)
		}
		if _, err := v.PutBlockBatch(t.Context(), tr.entries("H")); err != nil {
			t.Fatal(err)
		}
		if !has(t, store, front, tr["C"].Ref) {
			t.Fatal("the refill of H left C off the front")
		}
		requireKept(t, v, clk, store, tr, "D", "H and C's short list")
	})

	// oldU builds the root A, verified, and an unreached old block U -> K,
	// with U's list cut when cut is set.
	oldU := func(t *testing.T, cut bool) (*Volume, *testClock, kvtx.Store, testTree) {
		t.Helper()
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "K")
		tr.add(t, "U", "K")
		var cuts []string
		if cut {
			cuts = []string{"U"}
		}
		v, clk, store := oldStore(t, tr, "A", []string{"A", "K", "U"}, cuts...)
		verify(t, v, tr, "A")
		return v, clk, store, tr
	}

	t.Run("root onto an unverified block while Open", func(t *testing.T) {
		// A root moves onto the unreached U without a put. The hold pushes U
		// onto the front, which holds the flip; the walk decodes U, and K
		// stays. Before the hold nets, U's count is zero at a ripe sequence
		// the cursor passed, so only its dequeue by the Land keeps the free
		// proof from popping U.
		v, clk, store, tr := oldU(t, false)
		if err := rootOnto(t, v, tr, "U"); err != nil {
			t.Fatal(err)
		}
		if !has(t, store, front, tr["U"].Ref) {
			t.Fatal("the hold left U off the front")
		}
		if res := collectAll(t, v); res.Popped != 0 || !has(t, store, front, tr["U"].Ref) {
			t.Fatalf("collect before the hold nets = %+v, want U kept on the front", res)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Flipped || res.Popped != 0 {
			t.Fatalf("collect with U held on the front = %+v, want no pop or flip", res)
		}
		requireKept(t, v, clk, store, tr, "K", "U")
	})

	t.Run("born block's hold onto an unverified block while Open", func(t *testing.T) {
		// A put stores N, which holds the unreached U. The hold pushes U onto
		// the front. Before the hold nets, U's count is zero at a ripe
		// sequence the cursor passed, so only its dequeue by the Land, which
		// the hold onto a stored block at zero makes, keeps the free proof
		// from popping U.
		v, clk, store, tr := oldU(t, false)
		tr.add(t, "N", "U")
		roots := []block.NamedRoot{{Name: "n", Ref: tr["N"].Ref}}
		if err := v.SetBucketRoots(t.Context(), "b", tr.entries("N"), roots); err != nil {
			t.Fatal(err)
		}
		if !has(t, store, front, tr["U"].Ref) {
			t.Fatal("the hold left U off the front")
		}
		if res := collectAll(t, v); res.Popped != 0 || !has(t, store, front, tr["U"].Ref) {
			t.Fatalf("collect before the hold nets = %+v, want U kept on the front", res)
		}
		requireKept(t, v, clk, store, tr, "K", "U")
	})

	t.Run("front entry the free proof pops", func(t *testing.T) {
		// A root moves onto U and away again. Once its release nets and
		// ages, the free proof holds for U: the collector takes U off the
		// front unwalked, flips, and sweeps U and K, which nothing holds.
		v, clk, store, tr := oldU(t, false)
		if err := rootOnto(t, v, tr, "U"); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "main", Previous: tr["U"].Ref}}); err != nil {
			t.Fatal(err)
		}
		var total block_gc.CollectResult
		collectUntil(t, v, clk, func(res block_gc.CollectResult) bool {
			total.Popped += res.Popped
			total.Swept += res.Swept
			return res.Settled
		})
		if total.Popped != 1 || total.Swept != 2 || countPrefix(t, store, front) != 0 {
			t.Fatalf("settle popped %d and swept %d, want U popped and U and K swept", total.Popped, total.Swept)
		}
		requireStored(t, v, tr, true, "A")
		requireStored(t, v, tr, false, "U", "K")
		requireAudit(t, v, 0)
	})

	t.Run("hold of an unmarked block while Sweeping", func(t *testing.T) {
		// Once the generation flips, U and K are unmarked: the existence
		// check answers absent, a root onto U is refused as unmarked, which
		// asks the writer to put it again, and the sweep deletes both.
		v, clk, store, tr := oldU(t, false)
		var entered uint64
		res := collectUntil(t, v, clk, func(res block_gc.CollectResult) bool {
			entered += res.Unmarked
			return res.Flipped
		})
		if entered != 2 {
			t.Fatalf("flip = %+v after %d run entries, want U and K in the run", res, entered)
		}
		if found, err := v.GetBlockExists(t.Context(), tr["U"].Ref); err != nil || found {
			t.Fatalf("exists U while Sweeping = %v err %v, want absent", found, err)
		}
		err := rootOnto(t, v, tr, "U")
		var unmarked *block_gc.AbsentError
		if !errors.As(err, &unmarked) || !errors.Is(err, block.ErrRefsUnknown) || !unmarked.Ref.EqualsRef(tr["U"].Ref) {
			t.Fatalf("root onto the unmarked U err %v, want an AbsentError", err)
		}
		if res := collectUntil(t, v, clk, settled); res.Swept != 2 || has(t, store, run, nil) {
			t.Fatalf("sweep = %+v, want U and K swept and the run gone", res)
		}
		requireStored(t, v, tr, false, "U", "K")
		if gen := livenessGeneration(t, v); gen != block_gc.Generation_GENERATION_SETTLED {
			t.Fatalf("generation %v after the sweep, want Settled", gen)
		}
		requireAudit(t, v, 0)
	})

	t.Run("put of an unmarked block while Sweeping", func(t *testing.T) {
		// U's list lacks K. After the flip a put of U under main alone is
		// refused, since it holds the unmarked K; a put of both gives birth
		// to each again, and the sweep keeps them.
		v, clk, store, tr := oldU(t, true)
		collectUntil(t, v, clk, flipped)
		if err := rootOnto(t, v, tr, "U", "U"); !errors.Is(err, block.ErrRefsUnknown) {
			t.Fatalf("put of U alone err %v, want K refused as unmarked", err)
		}
		if err := rootOnto(t, v, tr, "U", "U", "K"); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"U", "K"} {
			if !has(t, store, run, tr[name].Ref) {
				t.Fatalf("the rebirth dropped %s's run entry before the sweep", name)
			}
			if found, err := v.GetBlockExists(t.Context(), tr[name].Ref); err != nil || !found {
				t.Fatalf("exists %s after its rebirth = %v err %v, want found", name, found, err)
			}
		}
		if res := collectUntil(t, v, clk, settled); res.Swept != 0 {
			t.Fatalf("sweep = %+v, want the reborn U and K kept", res)
		}
		requireStored(t, v, tr, true, "A", "U", "K")
		requireAudit(t, v, 0)
	})

	t.Run("fill of an unmarked block while Sweeping", func(t *testing.T) {
		// After the flip a read fills U and K, which gives birth to each
		// again, and a root moves onto U: the sweep keeps them both.
		v, clk, store, tr := oldU(t, false)
		collectUntil(t, v, clk, flipped)
		fill(t, v, tr, "U", "K")
		if found, err := v.GetBlockExists(t.Context(), tr["U"].Ref); err != nil || !found {
			t.Fatalf("exists U after its fill = %v err %v, want found", found, err)
		}
		if !has(t, store, run, tr["U"].Ref) || !has(t, store, run, tr["K"].Ref) {
			t.Fatal("the fill dropped a run entry before the sweep")
		}
		if err := rootOnto(t, v, tr, "U"); err != nil {
			t.Fatalf("root onto the filled U: %v", err)
		}
		var swept uint64
		collectUntil(t, v, clk, func(res block_gc.CollectResult) bool {
			swept += res.Swept
			return res.Settled
		})
		if swept != 0 {
			t.Fatalf("the sweep after the fill swept %d, want the reborn U and K kept", swept)
		}
		requireStored(t, v, tr, true, "U", "K")
	})

	t.Run("put meets the records of an earlier life", func(t *testing.T) {
		// U names K. An earlier life leaves U's records in one state: queued
		// while Settled, fill-born with its logs not netted, parked while
		// Open, or unmarked while Sweeping. Then a writer puts U and K
		// again, by a Land or by a fill and a root. Whatever records U's
		// earlier life left, the put must leave U reading, and the
		// collector must keep it.
		fresh := func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree) {
			t.Helper()
			tr := testTree{}
			tr.add(t, "A")
			tr.add(t, "K")
			tr.add(t, "U", "K")
			tr.add(t, "W")
			store := store_kvtx_inmem.NewStore()
			v, clk := openLivenessVolume(t, store, true)
			waitAdopted(t, v)
			return v, clk, store, tr
		}
		queuedKey := slices.Concat(livenessPrefix, []byte{'Q'})
		parkedKey := slices.Concat(livenessPrefix, []byte{'X'})
		states := []struct {
			name string
			// open builds the state and returns the Volume.
			open func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree)
			// settle collects the Volume to its end state.
			settle func(t *testing.T, v *Volume, clk *testClock, store kvtx.Store, tr testTree)
		}{{
			name: "queued while Settled",
			open: func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree) {
				// A pass with frees off nets the release, so U waits on the
				// free queue.
				v, clk, store, tr := fresh(t)
				roots := []block.NamedRoot{{Name: "main", Ref: tr["U"].Ref}, {Name: "w", Ref: tr["W"].Ref}}
				if err := v.SetBucketRoots(t.Context(), "b", tr.entries("K", "U", "W"), roots); err != nil {
					t.Fatal(err)
				}
				if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "main", Previous: tr["U"].Ref}}); err != nil {
					t.Fatal(err)
				}
				clk.Add(block_gc.DefaultGrace + time.Second)
				if res := netAll(t, v); countPrefix(t, store, queuedKey) == 0 {
					t.Fatalf("collect = %+v, want U queued", res)
				}
				requireStored(t, v, tr, true, "U")
				return v, clk, store, tr
			},
		}, {
			name: "fill-born with its logs not netted",
			open: func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree) {
				v, clk, store, tr := fresh(t)
				fill(t, v, tr, "U", "K")
				return v, clk, store, tr
			},
		}, {
			name: "parked while Open",
			open: func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree) {
				// W, never walked, holds the generation Open.
				tr := testTree{}
				tr.add(t, "A")
				tr.add(t, "K")
				tr.add(t, "U", "K")
				tr.add(t, "W")
				v, clk, store := oldStore(t, tr, "A", []string{"A", "K", "U", "W"})
				verify(t, v, tr, "A")
				if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "w", Ref: tr["W"].Ref}}); err != nil {
					t.Fatal(err)
				}
				if err := rootOnto(t, v, tr, "U"); err != nil {
					t.Fatal(err)
				}
				clk.Add(block_gc.DefaultGrace + time.Second)
				collectAll(t, v)
				if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "main", Previous: tr["U"].Ref}}); err != nil {
					t.Fatal(err)
				}
				for range 4 {
					clk.Add(block_gc.DefaultGrace + time.Second)
					res := collectAll(t, v)
					if res.Flipped {
						t.Fatalf("collect = %+v, want W to hold the flip", res)
					}
				}
				if countPrefix(t, store, parkedKey) == 0 {
					t.Fatal("U is not parked")
				}
				return v, clk, store, tr
			},
			settle: func(t *testing.T, v *Volume, clk *testClock, store kvtx.Store, tr testTree) {
				verify(t, v, tr, "W")
				settleAll(t, v, clk, store, tr)
			},
		}, {
			name: "unmarked while Sweeping",
			open: func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree) {
				v, clk, store, tr := oldU(t, false)
				collectUntil(t, v, clk, flipped)
				if !has(t, store, run, tr["U"].Ref) {
					t.Fatal("U is not in the run")
				}
				return v, clk, store, tr
			},
			settle: func(t *testing.T, v *Volume, clk *testClock, store kvtx.Store, tr testTree) {
				settleAll(t, v, clk, store, tr)
			},
		}}
		for _, st := range states {
			for _, byFill := range []bool{false, true} {
				name := st.name + " put by a Land"
				if byFill {
					name = st.name + " put by a fill"
				}
				t.Run(name, func(t *testing.T) {
					v, clk, store, tr := st.open(t)
					if byFill {
						fill(t, v, tr, "U", "K")
						if err := rootOnto(t, v, tr, "U"); err != nil {
							t.Fatal(err)
						}
					} else if err := rootOnto(t, v, tr, "U", "U", "K"); err != nil {
						t.Fatal(err)
					}
					requireStored(t, v, tr, true, "U", "K")
					// A pass just inside the put's grace period nets the records
					// before it, and must keep U while the put waits past the cursor.
					clk.Add(block_gc.DefaultGrace - time.Second)
					collectAll(t, v)
					requireStored(t, v, tr, true, "U", "K")
					if st.settle != nil {
						st.settle(t, v, clk, store, tr)
					}
					for range 4 {
						clk.Add(block_gc.DefaultGrace + time.Second)
						collectAll(t, v)
					}
					requireStored(t, v, tr, true, "U", "K")
					requireAudit(t, v, 0)
				})
			}
		}
	})

	t.Run("read-only open beside a writer that flips", func(t *testing.T) {
		// A writer with frees on flips the generation to Sweeping with the
		// old U unmarked, so U reads absent to a writer checking before it
		// puts. A read-only Volume on the same store, as the debug commands
		// open one, opened before the flip or after it, answers the same: a
		// copy that trusted it would skip U's put and name a block the sweep
		// then deletes. A Settled Volume never opens a generation again, so
		// a reader that opened Settled has nothing to hide.
		for _, before := range []bool{false, true} {
			t.Run("before="+strconv.FormatBool(before), func(t *testing.T) {
				v, clk, store, tr := oldU(t, false)
				openRO := func() *Volume {
					ro, err := NewVolume(t.Context(), "test/liveness", store_kvkey.NewDefaultKVKey(), readOnlyStore{store.(*store_kvtx_inmem.Store)}, &store_kvtx.Config{}, true, true, nil, nil)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = ro.Close() })
					return ro
				}
				var ro *Volume
				if before {
					ro = openRO()
				}
				collectUntil(t, v, clk, flipped)
				if !has(t, store, run, tr["U"].Ref) {
					t.Fatal("U is not in the run")
				}
				if !before {
					ro = openRO()
				}
				for _, vol := range []*Volume{v, ro} {
					if found, err := vol.GetBlockExists(t.Context(), tr["U"].Ref); err != nil || found {
						t.Fatalf("U exists=%v err %v on the read-only=%v Volume, want absent", found, err, vol.readOnly)
					}
					if found, err := vol.GetBlockExists(t.Context(), tr["A"].Ref); err != nil || !found {
						t.Fatalf("A exists=%v err %v on the read-only=%v Volume, want present", found, err, vol.readOnly)
					}
				}
			})
		}
	})

	t.Run("unwalked root seeds the flip", func(t *testing.T) {
		// The carried root names A, which no walk decoded. The flip pushes
		// A onto the front rather than flip, and the walk then decodes A.
		tr := testTree{}
		tr.add(t, "K")
		tr.add(t, "A", "K")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "K"})
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Flipped || res.Seeded != 1 || !has(t, store, front, tr["A"].Ref) {
			t.Fatalf("collect with A unwalked = %+v, want A seeded and no flip", res)
		}
		requireKept(t, v, clk, store, tr, "K", "the carried root and A")
	})

	t.Run("release just before the flip seeds it", func(t *testing.T) {
		// A is verified, and the carried root holds it. Just before the
		// flip's transaction, a session moves the carried root off A, so no
		// named root names A but a down pair past the cursor does, as a
		// reader that held A may still read it. The flip pushes A rather
		// than flip; once the release nets and ages, the free proof pops A,
		// and the sweep deletes it.
		tr := testTree{}
		tr.add(t, "K")
		tr.add(t, "U", "K")
		a := tr.add(t, "A", "U")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "U", "K"}, "A")
		var released bool
		probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
			if released || !has(t, store, run, nil) || countPrefix(t, store, front) != 0 {
				return
			}
			released = true
			if err := v.SetBucketRoots(t.Context(), "space", nil, []block.NamedRoot{{Name: "world", Previous: a}}); err != nil {
				t.Fatal(err)
			}
		}}
		c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
		var total block_gc.CollectResult
		for range 8 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			res, err := c.Collect(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			total.Seeded += res.Seeded
			if res.Flipped {
				break
			}
		}
		if !released || total.Seeded != 1 {
			t.Fatalf("released %v and seeded %d, want the release's down pair to seed A", released, total.Seeded)
		}
		collectUntil(t, v, clk, settled)
		requireStored(t, v, tr, false, "A", "U", "K")
		requireAudit(t, v, 0)
	})

	// holdersOf returns the netted holders of ref.
	holdersOf := func(t *testing.T, v *Volume, ref *block.BlockRef) uint64 {
		t.Helper()
		var n uint64
		err := v.readRecords(t.Context(), func(rec *block_gc.Records) error {
			var err error
			n, _, err = rec.GetHolders(t.Context(), ref)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	// passes collects n times, letting the grace period pass before each.
	passes := func(t *testing.T, v *Volume, clk *testClock, n int) {
		t.Helper()
		for range n {
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
		}
	}
	// deadU builds the root A, the old U -> D unreached, and main holding D,
	// which the walk verified, so D stays outside the run. U dies while Open,
	// and its free parks it, so U's list still holds D beside main. It
	// returns once the generation flips.
	deadU := func(t *testing.T) (*Volume, *testClock, kvtx.Store, testTree) {
		t.Helper()
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "D")
		tr.add(t, "U", "D")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "D", "U"})
		verify(t, v, tr, "A", "D")
		if err := rootOnto(t, v, tr, "D"); err != nil {
			t.Fatal(err)
		}
		collectUntil(t, v, clk, flipped)
		if has(t, store, run, tr["D"].Ref) || !has(t, store, run, tr["U"].Ref) {
			t.Fatal("the flip left D in the run or U out of it")
		}
		if n := holdersOf(t, v, tr["D"].Ref); n != 2 {
			t.Fatalf("D has %d holders at the flip, want main's and the parked U's", n)
		}
		return v, clk, store, tr
	}

	t.Run("pop on a count a rewrite made stale", func(t *testing.T) {
		// A -> P -> C -> D, with P's list cut, so adoption counts C at zero.
		// The walk decodes P, whose list gains C, which goes onto the front
		// and owes a recount. No pass may pop C on its stale count: once the
		// recount gives C P's holder, nothing would push C again, and the
		// sweep would delete it under the live P.
		tr := testTree{}
		tr.add(t, "D")
		tr.add(t, "C", "D")
		tr.add(t, "P", "C")
		tr.add(t, "A", "P")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "P", "C", "D"}, "P")
		verify(t, v, tr, "A")
		if res := verify(t, v, tr, "P"); res.Rewritten != 1 || !has(t, store, front, tr["C"].Ref) {
			t.Fatalf("decode of P = %+v, want its list rewritten and C on the front", res)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Popped != 0 {
			t.Fatalf("pass with a recount owed = %+v, want C kept on the front", res)
		}
		requireKept(t, v, clk, store, tr, "D", "P's rewritten list and C")
		requireStored(t, v, tr, true, "P", "C", "D")
	})

	t.Run("rebirth of a block whose free released its child", func(t *testing.T) {
		// After the flip a put gives birth again to U under another root.
		// The rebirth releases the old list's holder of D and the new list
		// gives one: D has main and the reborn U, and keeps U's once main
		// moves off.
		v, clk, _, tr := deadU(t)
		if err := v.SetBucketRoots(t.Context(), "b", tr.entries("U"), []block.NamedRoot{{Name: "other", Ref: tr["U"].Ref}}); err != nil {
			t.Fatal(err)
		}
		collectUntil(t, v, clk, settled)
		if n := holdersOf(t, v, tr["D"].Ref); n != 2 {
			t.Fatalf("D has %d holders after U's rebirth, want main and U", n)
		}
		if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "main", Previous: tr["D"].Ref}}); err != nil {
			t.Fatal(err)
		}
		passes(t, v, clk, 4)
		requireStored(t, v, tr, true, "U", "D")
		requireAudit(t, v, 0)
	})

	t.Run("fill up onto a dead unmarked block", func(t *testing.T) {
		// After the flip a read fills B naming the unmarked U, whose fill up
		// raises U before the sweep deletes it. The sweep releases the old
		// list's holder of D, and main still holds D.
		v, clk, _, tr := deadU(t)
		tr.add(t, "B", "U")
		fill(t, v, tr, "B")
		clk.Add(block_gc.DefaultGrace + time.Second)
		res := collectAll(t, v)
		if res.Swept == 0 {
			t.Fatalf("pass after the fill = %+v, want the sweep run", res)
		}
		if !res.Settled {
			collectUntil(t, v, clk, settled)
		}
		passes(t, v, clk, 4)
		if n := holdersOf(t, v, tr["D"].Ref); n != 1 {
			t.Fatalf("D has %d holders after the sweep, want main's", n)
		}
		requireStored(t, v, tr, true, "D")
		requireStored(t, v, tr, false, "U")
	})

	t.Run("root onto a dead verified parent after the flip", func(t *testing.T) {
		// The walk verified the unreached P, which names the old C -> E. P
		// dies while Open and parks, which puts it in the run, and its list
		// still holds C, so the proof cannot pop C and the walk verifies C
		// and E; the flip unmarks P alone, and the pass that flips may sweep
		// it too. A root onto P after the flip is refused, as unmarked or as
		// missing, the sweep deletes P and releases C, and the frees take C
		// and E, which nothing names.
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "E")
		tr.add(t, "C", "E")
		tr.add(t, "P", "C")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "E", "C", "P"})
		verify(t, v, tr, "A", "P")
		walkFront(t, v, store, tr)
		collectUntil(t, v, clk, flipped)
		if found, err := v.GetBlockExists(t.Context(), tr["P"].Ref); err != nil || found {
			t.Fatalf("the unmarked P exists=%v err %v, want absent", found, err)
		}
		err := rootOnto(t, v, tr, "P")
		var unmarked *block_gc.AbsentError
		switch {
		case errors.Is(err, block.ErrNotFound):
		case !errors.As(err, &unmarked) || !errors.Is(err, block.ErrRefsUnknown) || !unmarked.Ref.EqualsRef(tr["P"].Ref):
			t.Fatalf("root onto the unmarked P err %v, want an AbsentError or ErrNotFound", err)
		}
		if livenessGeneration(t, v) != block_gc.Generation_GENERATION_SETTLED {
			collectUntil(t, v, clk, settled)
		}
		passes(t, v, clk, 3)
		requireStored(t, v, tr, false, "P", "C", "E")
		if has(t, store, run, nil) {
			t.Fatal("the run outlived the settle")
		}
		requireAudit(t, v, 0)
	})

	t.Run("root onto a dead verified parent before its free", func(t *testing.T) {
		// As above, but the root moves onto P in the window between the flip
		// and P's free, while P is still stored. The park put P in the run,
		// so the flip unmarked it: the root is refused as unmarked, and the
		// sweep and the frees take all three. Were it accepted, its hold
		// would reach C and E, and they would have to stay.
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "E")
		tr.add(t, "C", "E")
		tr.add(t, "P", "C")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "E", "C", "P"})
		verify(t, v, tr, "A", "P")
		walkFront(t, v, store, tr)
		var tried bool
		var rootErr error
		probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
			if tried || livenessGeneration(t, v) != block_gc.Generation_GENERATION_SWEEPING {
				return
			}
			if _, found, err := v.GetBlock(t.Context(), tr["P"].Ref); err != nil || !found {
				return
			}
			tried = true
			rootErr = rootOnto(t, v, tr, "P")
		}}
		swapCollector(v, block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now}))
		collectUntil(t, v, clk, settled)
		if !tried {
			t.Fatal("the root never moved onto P between the flip and its free")
		}
		switch {
		case rootErr == nil:
			requireStored(t, v, tr, true, "P", "C", "E")
		case errors.Is(rootErr, block.ErrRefsUnknown):
			requireStored(t, v, tr, false, "P", "C", "E")
		default:
			t.Fatal(rootErr)
		}
		requireAudit(t, v, 0)
	})

	t.Run("rebirth of a block a fill up raised", func(t *testing.T) {
		// U -> K, unreached, dies while Open. After the flip a read fills B
		// naming U. Once B's fill up raises U, and before the sweep reaches
		// U, a put gives birth to U and K again under another root. When B's
		// fill ages out and B dies, the root still holds U.
		v, clk, store, tr := oldU(t, false)
		collectUntil(t, v, clk, flipped)
		tr.add(t, "B", "U")
		fill(t, v, tr, "B")
		fillUps := slices.Concat(livenessPrefix, []byte{'N'})
		var put bool
		probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
			if put || countPrefix(t, store, fillUps) != 0 || !has(t, store, run, tr["U"].Ref) {
				return
			}
			put = true
			if err := v.SetBucketRoots(t.Context(), "b", tr.entries("U", "K"), []block.NamedRoot{{Name: "other", Ref: tr["U"].Ref}}); err != nil {
				t.Fatal(err)
			}
		}}
		c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
		for range 8 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			for {
				res, err := c.Collect(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if !res.More {
					break
				}
			}
		}
		if !put {
			t.Fatal("the fill up never applied before the sweep")
		}
		requireStored(t, v, tr, true, "U", "K")
		requireAudit(t, v, 0)
	})

	t.Run("fill naming an unmarked block", func(t *testing.T) {
		// After the flip a read fills B, which names the unmarked C. C is
		// absent to every step from the flip on, so B names an absent child,
		// as any fill of part of a tree does: B's up goes to C's count, which
		// the sweep keeps when it deletes C's bytes. A rooted parent naming an
		// absent block has no claim on that block's children either, so the
		// outcome is the same whether the sweep ran before or after B arrived.
		// Once a later fill stores C again, C has B's holder alone, and goes
		// when B does.
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "C")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "C"})
		tr.add(t, "B", "C")
		verify(t, v, tr, "A")
		collectUntil(t, v, clk, flipped)
		if !has(t, store, run, tr["C"].Ref) {
			t.Fatal("the flip left C out of the run")
		}
		fill(t, v, tr, "B")
		if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "other", Ref: tr["B"].Ref}}); err != nil {
			t.Fatal(err)
		}
		collectUntil(t, v, clk, settled)
		requireStored(t, v, tr, false, "C")
		requireStored(t, v, tr, true, "B")
		fill(t, v, tr, "C")
		passes(t, v, clk, 4)
		requireStored(t, v, tr, true, "B", "C")
		if n := holdersOf(t, v, tr["C"].Ref); n != 1 {
			t.Fatalf("C has %d holders once filled again, want B's", n)
		}
		requireAudit(t, v, 0)
		if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "other", Previous: tr["B"].Ref}}); err != nil {
			t.Fatal(err)
		}
		passes(t, v, clk, 6)
		requireStored(t, v, tr, false, "B", "C")
		requireAudit(t, v, 0)
	})

	t.Run("readoption revives a parked block with its list", func(t *testing.T) {
		// A -> C -> F -> G, with C's list cut, so the adoption counts F at
		// zero. While Open the free parks F, which keeps its list and count,
		// and its list still holds G. The walk verifies C, whose list gains F
		// and owes a recount. The readoption counts F from C's rewritten list
		// and G from F's parked list, so the settle keeps both.
		tr := testTree{}
		tr.add(t, "G")
		tr.add(t, "F", "G")
		tr.add(t, "C", "F")
		tr.add(t, "A", "C")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "C", "F", "G"}, "C")

		// Collect until the free parks F.
		var parked uint64
		for i := 0; parked < 1; i++ {
			if i == 8 {
				t.Fatalf("the free parked %d blocks, want F", parked)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			res := collectAll(t, v)
			if res.Flipped || res.Freed != 0 {
				t.Fatalf("collect before the walk = %+v, want no flip or free", res)
			}
			parked += res.Parked
		}

		// Verify A and C, then settle through the readoption.
		if res := verify(t, v, tr, "A", "C"); res.Rewritten != 1 {
			t.Fatalf("decode of A and C = %+v, want C's list rewritten", res)
		}
		res := settleAll(t, v, clk, store, tr)
		requireStored(t, v, tr, true, "A", "C", "F", "G")
		for _, name := range []string{"F", "G"} {
			if n := holdersOf(t, v, tr[name].Ref); n != 1 {
				t.Fatalf("%s has %d holders after the settle (%+v), want 1", name, n, res)
			}
		}
		requireAudit(t, v, 0)
	})

	t.Run("readoption after a pop with a writer ahead of the cursor", func(t *testing.T) {
		// The free proof pops U while the unverified seed A holds the flip,
		// and a readoption then recounts, which gives U and K new ripe
		// sequences, and the free parks both. Root d holds the old D through
		// the recount; its release nets with frees off, so D is Dead on the
		// free queue and not parked. Root e then moves onto the Dead D, whose
		// Land takes D off the free queue until that move nets, so the free
		// cannot park D. The walk verifies A, and the flip must not wait for
		// D: D died after the recount, so it names no popped child.
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "K")
		tr.add(t, "U", "K")
		tr.add(t, "D")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "K", "U", "D"})
		ctx := t.Context()
		d := []block.NamedRoot{{Name: "d", Ref: tr["D"].Ref}}
		if err := v.SetBucketRoots(ctx, "b", nil, d); err != nil {
			t.Fatal(err)
		}
		if err := rootOnto(t, v, tr, "U"); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: tr["U"].Ref}}); err != nil {
			t.Fatal(err)
		}
		var popped uint64
		res := collectUntil(t, v, clk, func(res block_gc.CollectResult) bool {
			popped += res.Popped
			return popped != 0 || res.Flipped
		})
		if res.Flipped || has(t, store, front, tr["U"].Ref) {
			t.Fatalf("collect = %+v, want U popped and the flip held by A", res)
		}

		// Readopt, and let the free park what the recount left at zero.
		frees := livenessState(t, v).GetFrees()
		for _, on := range []bool{!frees, frees} {
			if _, err := block_gc.SetFrees(ctx, collectorStore{v: v}, on); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			if res := collectAll(t, v); res.Flipped {
				t.Fatalf("collect flipped with A unverified: %+v", res)
			}
		}

		// Release D, net the release with frees off, then move root e onto
		// the Dead D and walk A and D.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "d", Previous: tr["D"].Ref}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		netAll(t, v)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "e", Ref: tr["D"].Ref}}); err != nil {
			t.Fatal(err)
		}
		verify(t, v, tr, "A", "D")

		// Root e's move nets, and D, off the queue, stays; the flip comes.
		if res := collectAll(t, v); !res.Flipped {
			t.Fatalf("collect with D held by root e = %+v, want the flip", res)
		}
		settleAll(t, v, clk, store, tr)
		requireStored(t, v, tr, true, "A", "D")
		requireStored(t, v, tr, false, "U", "K")
		requireAudit(t, v, 0)
	})

	t.Run("front entry whose list and bytes are lost", func(t *testing.T) {
		// Root main moves onto the old U, which puts U on the front, and its
		// release nets with frees off, so U is Dead on the front and not
		// popped. Damage then loses U's list: while its
		// bytes remain, U stays on the front for the walk to decode. Once the
		// bytes are lost too nothing can decode U, so the free proof must pop
		// it, and the flip comes with A kept, rather than U keeping the
		// generation Open.
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "K")
		tr.add(t, "U", "K")
		v, clk, store := oldStore(t, tr, "A", []string{"A", "K", "U"})
		ctx := t.Context()
		if err := rootOnto(t, v, tr, "U"); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: tr["U"].Ref}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := netAll(t, v); !has(t, store, front, tr["U"].Ref) {
			t.Fatalf("netting = %+v, want U Dead on the front", res)
		}

		// Lose U's list: while its bytes remain, U stays once it ripens.
		lose := func(f func(p *physicalTx) error) {
			if err := v.withDirectAtomic(ctx, f); err != nil {
				t.Fatal(err)
			}
		}
		lose(func(p *physicalTx) error { return p.rec.DeleteChildren(ctx, tr["U"].Ref) })
		verify(t, v, tr, "A")
		clk.Add(block_gc.DefaultGrace)
		if res := collectAll(t, v); res.Flipped || !has(t, store, front, tr["U"].Ref) {
			t.Fatalf("collect = %+v, want U on the front while its bytes remain", res)
		}

		// Lose U's bytes too, and collect until the flip.
		lose(func(p *physicalTx) error { return p.blocks.RmBlock(ctx, tr["U"].Ref) })
		res := collectUntil(t, v, clk, func(res block_gc.CollectResult) bool { return res.Flipped })
		if has(t, store, front, tr["U"].Ref) {
			t.Fatalf("collect = %+v, want U off the front", res)
		}
		settleAll(t, v, clk, store, tr)
		requireStored(t, v, tr, true, "A")
		requireStored(t, v, tr, false, "U")
	})

	t.Run("put of a lost front entry beside its pop", func(t *testing.T) {
		// As "front entry whose list and bytes are lost", U is Dead on the
		// front with its list and bytes lost, so the next pass pops
		// and parks it. A writer puts U and K again and moves root main onto
		// U, just before that pass, just after it, or once the flip left U
		// unmarked. The put finds no list, yet U keeps its zero count, its
		// run entry and, after the pass, its parked key. The store must keep
		// U, which root main holds, through the flip and the sweep.
		for _, name := range []string{"before the pop", "after the pop", "after the flip"} {
			t.Run(name, func(t *testing.T) {
				tr := testTree{}
				tr.add(t, "A")
				tr.add(t, "K")
				tr.add(t, "U", "K")
				v, clk, store := oldStore(t, tr, "A", []string{"A", "K", "U"})
				ctx := t.Context()
				if err := rootOnto(t, v, tr, "U"); err != nil {
					t.Fatal(err)
				}
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: tr["U"].Ref}}); err != nil {
					t.Fatal(err)
				}
				clk.Add(block_gc.DefaultGrace + time.Second)
				if res := netAll(t, v); !has(t, store, front, tr["U"].Ref) {
					t.Fatalf("netting = %+v, want U Dead on the front", res)
				}

				// Lose U's list and bytes, and walk A, so U alone holds the flip.
				err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
					if err := p.rec.DeleteChildren(ctx, tr["U"].Ref); err != nil {
						return err
					}
					return p.blocks.RmBlock(ctx, tr["U"].Ref)
				})
				if err != nil {
					t.Fatal(err)
				}
				verify(t, v, tr, "A")
				clk.Add(block_gc.DefaultGrace)

				// Put U again and hold it, beside the pass that pops it.
				put := func() {
					t.Helper()
					if err := rootOnto(t, v, tr, "U", "U", "K"); err != nil {
						t.Fatal(err)
					}
				}
				switch name {
				case "before the pop":
					put()
				case "after the pop":
					if res := collectAll(t, v); res.Popped == 0 {
						t.Fatalf("collect = %+v, want U popped", res)
					}
					put()
				default:
					if res := collectUntil(t, v, clk, flipped); res.Swept != 0 {
						t.Fatalf("collect = %+v, want the flip before the sweep", res)
					}
					put()
				}
				collectUntil(t, v, clk, settled)
				requireStored(t, v, tr, true, "A", "U", "K")
				requireAudit(t, v, 0)
			})
		}
	})

	t.Run("put again of a swept child before its unmarked parent", func(t *testing.T) {
		// The old chain G -> P -> C is garbage, among 300 old leaves, and C's
		// key sorts more than a sweep slice before P's. Between the slice
		// that sweeps C and the one that sweeps P, a put stores C again under
		// the new X, in one batch, so C counts X's implied holder alone. The
		// sweep of P must not release a holder from C, which X holds. In the
		// zero count case C's count reads zero after the sweep, which only
		// damage leaves, since netting floors an unlisted block at its
		// implied holder, so the put reparks C, and the repark must keep C's
		// run entry: P's old list still holds C, and P's
		// sweep reads that entry to keep C's holder.
		for _, zero := range []bool{false, true} {
			t.Run("zero count "+strconv.FormatBool(zero), func(t *testing.T) {
				var tr testTree
				listed := []string{"A", "G", "P", "C"}
				for i := 0; ; i++ {
					if i == 64 {
						t.Fatal("no chain sorts C a slice before P")
					}
					tr = testTree{}
					tr.add(t, "A")
					fillers := make([][]byte, 300)
					for j := range fillers {
						fillers[j] = block_gc.BlockKey(tr.add(t, "F"+strconv.Itoa(j)))
					}
					tr.add(t, "C"+strconv.Itoa(i))
					tr["C"] = tr["C"+strconv.Itoa(i)]
					c := block_gc.BlockKey(tr["C"].Ref)
					p := block_gc.BlockKey(tr.add(t, "P", "C"))
					between := 0
					for _, k := range fillers {
						if bytes.Compare(c, k) < 0 && bytes.Compare(k, p) < 0 {
							between++
						}
					}
					if between > 130 {
						break
					}
				}
				for j := range 300 {
					listed = append(listed, "F"+strconv.Itoa(j))
				}
				tr.add(t, "G", "P")
				tr.add(t, "X", "C")
				v, clk, store := oldStore(t, tr, "A", listed)
				verify(t, v, tr, "A")

				// Put C again under X once the sweep took C and not yet P.
				runValue := func(ref *block.BlockRef) string {
					tx, err := store.NewTransaction(t.Context(), false)
					if err != nil {
						t.Fatal(err)
					}
					defer tx.Discard()
					value, _, err := tx.Get(t.Context(), slices.Concat(run, block_gc.BlockKey(ref)))
					if err != nil {
						t.Fatal(err)
					}
					return string(value)
				}
				var put bool
				probe := &walkProbe{Store: collectorStore{v: v}, before: func() {
					if put || livenessGeneration(t, v) != block_gc.Generation_GENERATION_SWEEPING {
						return
					}
					if has(t, store, run, tr["C"].Ref) != zero || !has(t, store, run, tr["P"].Ref) {
						return
					}
					if zero && (runValue(tr["C"].Ref) != "s" || runValue(tr["P"].Ref) != "") {
						return
					}
					put = true
					if zero {
						err := v.withDirectAtomic(t.Context(), func(p *physicalTx) error {
							return p.rec.SetHolders(t.Context(), tr["C"].Ref, 0, 0)
						})
						if err != nil {
							t.Fatal(err)
						}
					}
					if err := v.SetBucketRoots(t.Context(), "b", tr.entries("C", "X"), []block.NamedRoot{{Name: "x", Ref: tr["X"].Ref}}); err != nil {
						t.Fatal(err)
					}
				}}
				c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
				for range 12 {
					clk.Add(block_gc.DefaultGrace + time.Second)
					for more := true; more; {
						res, err := c.Collect(t.Context())
						if err != nil {
							t.Fatal(err)
						}
						more = res.More
					}
				}
				if !put || livenessGeneration(t, v) != block_gc.Generation_GENERATION_SETTLED {
					t.Fatalf("put %v at generation %v, want C put between the slices and the settle done", put, livenessGeneration(t, v))
				}
				requireStored(t, v, tr, true, "X", "C")
				requireStored(t, v, tr, false, "G", "P")
				if n := holdersOf(t, v, tr["C"].Ref); n != 1 {
					t.Fatalf("C has %d holders, want X's", n)
				}
				requireAudit(t, v, 0)
			})
		}
	})

	t.Run("put of an unmarked block that lost its list", func(t *testing.T) {
		// The old chain P -> U is garbage, so the flip unmarks both, and
		// U's count keeps P's holder. U's list is then
		// lost, and a Land, or a fill and a root, stores U again under main
		// while Sweeping: the sweep must keep U, which main holds.
		for _, byFill := range []bool{false, true} {
			tr := testTree{}
			tr.add(t, "A")
			tr.add(t, "U")
			tr.add(t, "P", "U")
			v, clk, _ := oldStore(t, tr, "A", []string{"A", "P", "U"})
			verify(t, v, tr, "A")
			collectUntil(t, v, clk, flipped)
			err := v.withDirectAtomic(t.Context(), func(p *physicalTx) error {
				return p.rec.DeleteChildren(t.Context(), tr["U"].Ref)
			})
			if err != nil {
				t.Fatal(err)
			}
			if byFill {
				fill(t, v, tr, "U")
				err = rootOnto(t, v, tr, "U")
			} else {
				err = rootOnto(t, v, tr, "U", "U")
			}
			if err != nil {
				t.Fatal(err)
			}
			collectUntil(t, v, clk, settled)
			requireStored(t, v, tr, true, "A", "U")
			requireAudit(t, v, 0)
		}
	})

	t.Run("renewal of a lapsed pin on an unmarked block", func(t *testing.T) {
		// A pin holds the old U, and its process stalls past the lease, so
		// the collector releases the pin's root. The process stays stalled
		// while the release nets a grace period later, since a renewal
		// before then holds U again, as U still counts the pin. U dies, the
		// free proof pops it, and the flip unmarks it. The process then pins the young W and
		// renews: the renewal must drop U's pin, since nothing holds U, and
		// renew W's, rather than fail the renewal on U.
		v, clk, store, tr := oldU(t, false)
		ctx := t.Context()
		if _, err := v.PinBucketRoot(ctx, tr["U"].Ref); err != nil {
			t.Fatal(err)
		}
		if !has(t, store, front, tr["U"].Ref) {
			t.Fatal("the pin left U off the front")
		}
		clk.Add(rootLease + time.Second)
		if res, err := v.collector.Collect(ctx); err != nil || res.Lapsed != 1 {
			t.Fatalf("stalled pass lapsed %d err %v, want U's pin", res.Lapsed, err)
		}
		for pass := 0; ; pass++ {
			if pass == 16 {
				t.Fatalf("the stalled collection never flipped; the generation is %v", livenessGeneration(t, v))
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			res, err := v.collector.Collect(ctx)
			for err == nil && res.More && !res.Flipped {
				res, err = v.collector.Collect(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.Flipped {
				break
			}
		}

		// Pin W, then renew both pins.
		w := tr.add(t, "W")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("W"), []block.NamedRoot{{Name: "w", Ref: w}}); err != nil {
			t.Fatal(err)
		}
		if _, err := v.PinBucketRoot(ctx, w); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "w", Previous: w}}); err != nil {
			t.Fatal(err)
		}
		if _, err := v.renewRoots(ctx); err != nil {
			t.Fatalf("renewal with U's pin lapsed onto the unmarked U: %v", err)
		}
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "W")
		requireStored(t, v, tr, false, "U", "K")
	})

	t.Run("writers across the settle on SQLite", func(t *testing.T) {
		// B<i> -> C<i> -> D<i> for 150 i, with each C's list cut, on a
		// SQLite store, whose readers do not wait on its writer. Two writers
		// run through the whole settle. One commits after every collector
		// transaction, so between every pair of slices and across the flip:
		// it roots a new name onto B<step mod 50>, putting its chain again, and
		// releases the name it rooted five steps before. The other runs
		// beside the collector, rooting c<k> onto B<k+50> and, for even k,
		// e<k> onto B<k>. Every block a held root reaches stays, the audit
		// is clean, and B, C and D from 100 to 139, which nothing roots, are
		// swept.
		ctx := t.Context()
		store, err := store_kvtx_sqlite.Open(ctx, filepath.Join(t.TempDir(), "volume.db"), "kv")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		})
		tr := testTree{}
		listed, cut := pairs(t, tr, 150)
		v, clk := oldStoreOn(t, store, tr, "A", listed, cut...)
		verify(t, v, tr, "A")
		reached := func(b int) []string {
			s := strconv.Itoa(b)
			return []string{"B" + s, "C" + s, "D" + s}
		}
		// setRoot roots name onto B<b>, putting B, C and D again: a chain
		// let go before the flip is unmarked, and a put of B alone would
		// name a C the sweep deletes, as a put naming a child never stored.
		setRoot := func(name string, b int) error {
			names := reached(b)
			return v.SetBucketRoots(ctx, "b", tr.entries(names...), []block.NamedRoot{{Name: name, Ref: tr[names[0]].Ref}})
		}

		// The paced writer: step's root holds B<step mod 50>, and the root
		// five steps back is released. While Sweeping, it also roots r<j>
		// onto B<140+j>, unmarked at the flip, putting B, C and D again.
		var steps, reborn int
		probe := &paceProbe{Store: collectorStore{v: v}, limit: 1 << 20, after: func() {
			requireFront(t, v)
			if err := setRoot("s"+strconv.Itoa(steps), steps%50); err != nil {
				t.Fatal(err)
			}
			if reborn < 10 && livenessGeneration(t, v) == block_gc.Generation_GENERATION_SWEEPING {
				names := reached(140 + reborn)
				err := v.SetBucketRoots(ctx, "b", tr.entries(names...), []block.NamedRoot{{Name: "r" + strconv.Itoa(reborn), Ref: tr[names[0]].Ref}})
				if err != nil {
					t.Fatal(err)
				}
				reborn++
			}
			if old := steps - 5; old >= 0 {
				err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "s" + strconv.Itoa(old), Previous: tr["B"+strconv.Itoa(old%50)].Ref}})
				if err != nil {
					t.Fatal(err)
				}
			}
			steps++
		}}

		// The concurrent writer.
		var wg sync.WaitGroup
		var werr error
		wg.Go(func() {
			for k := range 50 {
				if werr = setRoot("c"+strconv.Itoa(k), k+50); werr != nil {
					return
				}
				if k%2 == 0 {
					if werr = setRoot("e"+strconv.Itoa(k), k); werr != nil {
						return
					}
				}
				time.Sleep(time.Millisecond)
			}
		})

		// Settle through the paced store.
		c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
		var flipped, settled bool
		var freed, swept uint64
		for round := 0; livenessGeneration(t, v) != block_gc.Generation_GENERATION_SETTLED; round++ {
			if round == 32 {
				t.Fatalf("the old generation never settled: it is %v", livenessGeneration(t, v))
			}
			if livenessGeneration(t, v) == block_gc.Generation_GENERATION_OPEN && livenessAdoption(t, v) == block_gc.Adoption_ADOPTION_ADOPTED {
				walkFront(t, v, store, tr)
			}
			if livenessAdoption(t, v) == block_gc.Adoption_ADOPTION_UNADOPTED {
				if _, err := v.Adopt(ctx); err != nil {
					t.Fatal(err)
				}
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			for {
				res, err := c.Collect(ctx)
				if err != nil {
					t.Fatal(err)
				}
				flipped = flipped || res.Flipped
				settled = settled || res.Settled
				freed += res.Freed
				swept += res.Swept
				if !res.More {
					break
				}
			}
		}
		wg.Wait()
		if werr != nil {
			t.Fatal(werr)
		}
		if !flipped || !settled {
			t.Fatalf("flipped %v, settled %v, want both", flipped, settled)
		}
		t.Logf("settled in %d collector transactions, %d swept, %d freed, %d rooted while Sweeping", steps, swept, freed, reborn)
		if reborn == 0 {
			t.Fatal("no writer rooted an unmarked block while Sweeping")
		}

		// Every held root's blocks stay; nothing rooted past 100 does.
		var held []string
		for k := range 50 {
			held = append(held, reached(k+50)...)
			if k%2 == 0 {
				held = append(held, reached(k)...)
			}
		}
		for step := max(steps-5, 0); step < steps; step++ {
			held = append(held, reached(step%50)...)
		}
		for j := range reborn {
			held = append(held, reached(140+j)...)
		}
		requireStored(t, v, tr, true, held...)
		for b := 100; b < 140; b++ {
			requireStored(t, v, tr, false, reached(b)...)
		}

		// The paced writer's last releases follow the collector's last
		// transaction; net them, with the writers stopped, before the audit.
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireAudit(t, v, 0)
	})

	t.Run("frees off", func(t *testing.T) {
		// With frees off the collector runs none of the settle: the run is
		// never built, nothing flips, and every block stays.
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "U")
		store := store_kvtx_inmem.NewStore()
		writeLegacyStore(t, store, store_kvkey.NewDefaultKVKey(), tr, "A", true, "A", "U")
		v, clk := openLivenessVolume(t, store, false)
		if _, err := v.Adopt(t.Context()); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Unmarked != 0 || res.Flipped || res.Freed != 0 {
			t.Fatalf("collect with frees off = %+v, want no settle", res)
		}
		if gen := livenessGeneration(t, v); gen != block_gc.Generation_GENERATION_OPEN || countPrefix(t, store, run) != 0 {
			t.Fatalf("generation %v with %d run entries, want Open and no run", gen, countPrefix(t, store, run))
		}
		requireStored(t, v, tr, true, "A", "U")
	})
}

// requireFront fails t when, while v's old generation is Open, a block a
// young or verified list names is unverified, held and off the front, as
// block_gc.AuditFront checks.
func requireFront(t *testing.T, v *Volume) {
	t.Helper()
	err := v.readRecords(context.Background(), func(rec *block_gc.Records) error {
		missing, sample, err := block_gc.AuditFront(context.Background(), rec)
		if err == nil && missing != 0 {
			t.Errorf("%d held unverified blocks a marked list names are off the front, such as %s", missing, sample[0].MarshalString())
		}
		return err
	})
	if err != nil {
		t.Error(err)
	}
}

// livenessGeneration returns the generation of v's old blocks.
func livenessGeneration(t *testing.T, v *Volume) block_gc.Generation {
	t.Helper()
	var gen block_gc.Generation
	err := v.readRecords(t.Context(), func(rec *block_gc.Records) error {
		state, err := rec.GetState(t.Context())
		gen = state.GetGeneration()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return gen
}

// setGeneration sets the generation of v's old blocks, as an upgrade from
// format 6 sets it Open.
func setGeneration(t *testing.T, v *Volume, gen block_gc.Generation) {
	t.Helper()
	err := collectorStore{v: v}.Update(t.Context(), func(rec *block_gc.Records, _ block.StoreOps) error {
		state, err := rec.GetState(t.Context())
		if err != nil {
			return err
		}
		state.Generation = gen
		return rec.SetState(t.Context(), state)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// walkFront decodes, from the blocks of tr, each block on the settle's front
// until the front empties or holds only blocks the walk refused, as the
// background walk does, and returns the rebuilds' totals.
func walkFront(t *testing.T, v *Volume, store kvtx.Store, tr testTree) block_gc.ListRebuildResult {
	t.Helper()
	byKey := make(map[string]*block.PutBatchEntry, len(tr))
	for _, entry := range tr {
		byKey[string(block_gc.BlockKey(entry.Ref))] = entry
	}
	prefix := slices.Concat(livenessPrefix, []byte{'G'})
	var total block_gc.ListRebuildResult
	for range 64 {
		var keys [][]byte
		tx, err := store.NewTransaction(t.Context(), false)
		if err != nil {
			t.Fatal(err)
		}
		err = tx.ScanPrefixKeys(t.Context(), prefix, func(key []byte) error {
			keys = append(keys, bytes.Clone(key[len(prefix):]))
			return nil
		})
		tx.Discard()
		if err != nil {
			t.Fatal(err)
		}
		if len(keys) == 0 {
			return total
		}
		rb, err := v.OpenListRebuild(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range keys {
			entry := byKey[string(key)]
			if entry == nil {
				t.Fatalf("the front names %x, a block the tree lacks", key)
			}
			if err := rb.Verify(t.Context(), entry.Ref, entry.Refs); err != nil {
				t.Fatal(err)
			}
		}
		res, err := rb.Finish(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		total.Verified += res.Verified
		total.Rewritten += res.Rewritten
		total.Dropped += res.Dropped
		total.Front = res.Front
		if res.Dropped != 0 && res.Front == uint64(len(keys)) {
			return total
		}
	}
	t.Fatal("the walk never emptied the front")
	return total
}

// settleAll settles v's old generation: it walks the front, recounts an
// unadopted Volume, lets the grace period pass and collects, until the
// generation settles, and returns the passes' totals.
func settleAll(t *testing.T, v *Volume, clk *testClock, store kvtx.Store, tr testTree) block_gc.CollectResult {
	t.Helper()
	var total block_gc.CollectResult
	for range 32 {
		gen := livenessGeneration(t, v)
		if gen == block_gc.Generation_GENERATION_SETTLED {
			return total
		}
		if gen == block_gc.Generation_GENERATION_OPEN && livenessAdoption(t, v) == block_gc.Adoption_ADOPTION_ADOPTED {
			walkFront(t, v, store, tr)
		}
		if livenessAdoption(t, v) == block_gc.Adoption_ADOPTION_UNADOPTED {
			if _, err := v.Adopt(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
		requireFront(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		res := collectAll(t, v)
		total.Freed += res.Freed
		total.Parked += res.Parked
		total.Requeued += res.Requeued
		total.Unmarked += res.Unmarked
		total.Popped += res.Popped
		total.Seeded += res.Seeded
		total.Swept += res.Swept
		total.Flipped = total.Flipped || res.Flipped
		total.Unadopted = total.Unadopted || res.Unadopted
		total.Settled = total.Settled || res.Settled
	}
	t.Fatalf("the old generation never settled: it is %v", livenessGeneration(t, v))
	return total
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

// markUnadopted returns an adopted Volume to unadopted, as one that predates
// readOnlyStore is an in-memory store that reports itself read-only, as an
// s4db opened for reading does, so a Volume over it starts no writer. It
// keeps the store's atomic commits, which a Volume needs for liveness.
type readOnlyStore struct {
	*store_kvtx_inmem.Store
}

// ReadOnly reports that the store refuses every write transaction.
func (readOnlyStore) ReadOnly() bool {
	return true
}

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

// Update calls before, then runs the transaction.
func (p *walkProbe) Update(ctx context.Context, fn func(rec *block_gc.Records, blocks block.StoreOps) error) error {
	p.before()
	return p.Store.Update(ctx, fn)
}

// livenessRecords returns the liveness records of the kinds in store, by key.
func livenessRecords(t *testing.T, store kvtx.Store, kinds ...byte) map[string]string {
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

// newAdmitVolume opens a Volume with frees on, so the check refuses, and
// builds admitTree in it. Without frees it then swaps in a collector that
// frees nothing, keeping the refusal.
func newAdmitVolume(t *testing.T, frees bool) (*Volume, *testClock, testTree, func(extra ...string) error) {
	t.Helper()
	v, clk := newLivenessVolume(t, true)
	if !frees {
		v.collector = v.newCollector(collectorStore{v: v}, false, block_gc.DefaultGrace)
	}
	tr, admit := admitTree(t, v)
	return v, clk, tr, admit
}

// admitTree sets root payloads of v onto Q and builds the payload tree
// P -> L and H naming P and Q, none of them stored. admit moves payloads
// from Q onto H, storing H and the extra blocks and requiring its blocks
// held.
func admitTree(t *testing.T, v *Volume) (testTree, func(extra ...string) error) {
	t.Helper()
	tr := testTree{}
	q := tr.add(t, "Q")
	if err := v.SetBucketRoots(t.Context(), "b", tr.entries("Q"), []block.NamedRoot{{Name: "payloads", Ref: q}}); err != nil {
		t.Fatal(err)
	}
	tr.add(t, "L")
	tr.add(t, "P", "L")
	h := tr.add(t, "H", "P", "Q")
	admit := func(extra ...string) error {
		roots := []block.NamedRoot{{Name: "payloads", Ref: h, Previous: q, RequireHeld: true}}
		return v.SetBucketRoots(t.Context(), "b", tr.entries(append([]string{"H"}, extra...)...), roots)
	}
	return tr, admit
}

// freeUnderDeadParent stores P -> L under a stage, releases it, nets the
// release with frees off, puts P again and nets that too, so P stays stored,
// listed and Dead on the free queue. P holds L until its free, so L stays
// stored, listed and held.
func freeUnderDeadParent(t *testing.T, v *Volume, clk *testClock, tr testTree) {
	t.Helper()
	ctx := t.Context()
	stage, release, err := v.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stagePut(t, v, stage, tr, "L", "P")
	release()
	if err := v.withDirectAtomic(ctx, nil); err != nil {
		t.Fatal(err)
	}
	clk.Add(block_gc.DefaultGrace + time.Second)
	netAll(t, v)
	if _, err := v.PutBlockBatch(ctx, tr.entries("P")); err != nil {
		t.Fatal(err)
	}

	// Net the put again, which queues P at zero again.
	clk.Add(block_gc.DefaultGrace + time.Second)
	netAll(t, v)
	requireStored(t, v, tr, true, "P")
	if listed, held := admitRecords(t, v, tr, "P"); !listed || held {
		t.Fatalf("Dead P listed=%v held=%v, want listed and not held", listed, held)
	}
	requireStored(t, v, tr, true, "L")
	if listed, held := admitRecords(t, v, tr, "L"); !listed || !held {
		t.Fatalf("L under Dead P listed=%v held=%v, want both", listed, held)
	}
	t.Log("P stored, listed and not held; it still holds L")
}

// requireDeadParentAdmit checks the admit over P, Dead after
// freeUnderDeadParent: P holds L until its free, so the admit lands, and the
// admitted tree stays stored whole once a collector that frees nets it.
func requireDeadParentAdmit(t *testing.T, v *Volume, clk *testClock, tr testTree, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("admit over L held by Dead P = %v, want commit", err)
	}
	if got, err := v.GetBucketRoot(t.Context(), "b", "payloads"); err != nil || !got.EqualsRef(tr["H"].Ref) {
		t.Fatalf("payloads holds %v after the admit: %v", got, err)
	}
	requireAdmitKept(t, v, clk, tr)
}

// stagePut puts each named block of tr under stage, one batch each.
func stagePut(t *testing.T, v *Volume, stage string, tr testTree, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := v.PrepareStagedBlockBatch(t.Context(), stage, tr.entries(name)); err != nil {
			t.Fatal(err)
		}
	}
}

// buildAndRelease puts L and P under a stage, releases it, and nets the
// release past the grace period.
func buildAndRelease(t *testing.T, v *Volume, clk *testClock, tr testTree) {
	t.Helper()
	stage, release, err := v.OpenStage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	stagePut(t, v, stage, tr, "L", "P")
	release()
	if err := v.withDirectAtomic(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	clk.Add(block_gc.DefaultGrace + time.Second)
	collectAll(t, v)
}

// admitRecords reports whether the named block of tr has a child list and
// whether the records hold it.
func admitRecords(t *testing.T, v *Volume, tr testTree, name string) (listed, held bool) {
	t.Helper()
	ref := tr[name].Ref
	err := v.withDirectAtomic(t.Context(), func(p *physicalTx) error {
		var err error
		if listed, err = p.rec.HasChildren(t.Context(), ref); err != nil {
			return err
		}
		held, err = p.rec.Held(t.Context(), ref)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return listed, held
}

// requireNotHeld checks that err refuses the admit and payloads still holds
// Q.
func requireNotHeld(t *testing.T, v *Volume, tr testTree, err error) {
	t.Helper()
	if !errors.Is(err, block.ErrRootNotHeld) {
		t.Fatalf("admit = %v, want ErrRootNotHeld", err)
	}
	if got, err := v.GetBucketRoot(t.Context(), "b", "payloads"); err != nil || !got.EqualsRef(tr["Q"].Ref) {
		t.Fatalf("payloads holds %v after the refusal: %v", got, err)
	}
}

// requireAdmitKept turns frees on, nets every released stage past the grace
// period, and checks that the admitted tree is stored whole.
func requireAdmitKept(t *testing.T, v *Volume, clk *testClock, tr testTree) {
	t.Helper()
	swapCollector(v, v.newCollector(collectorStore{v: v}, true, block_gc.DefaultGrace))
	if err := v.withDirectAtomic(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
	}
	requireStored(t, v, tr, true, "H", "P", "L", "Q")
	requireAudit(t, v, 0)
}

// setUnadopted marks store, which must be empty, as a Volume of the current
// format that predates adoption.
func setUnadopted(t *testing.T, store kvtx.Store) {
	t.Helper()
	keys := store_kvkey.NewDefaultKVKey()
	state := &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_UNADOPTED}
	if err := writeFormat(t.Context(), store, keys, state, nil); err != nil {
		t.Fatal(err)
	}
}

// TestFormatUpgradeCarriesLiveness opens a store of format 6 and checks that
// format 7 keeps its child lists in place, carries its named roots into the
// root table, leaves it unadopted, sweeps the rest of the old graph, and that
// an adoption then counts its holders. A second store whose root names a block
// stored without a child list refuses adoption.
// TestObsoleteKindsSpareLiveKinds fails when a kind a format upgrade
// deletes is one format 10 writes, since the upgrade would delete its live
// records.
func TestObsoleteKindsSpareLiveKinds(t *testing.T) {
	for _, c := range []struct {
		name  string
		kinds []byte
	}{
		{"legacyGraphKinds", legacyGraphKinds},
		{"Format9Kinds", block_gc.Format9Kinds},
	} {
		for _, kind := range c.kinds {
			if slices.Contains(block_gc.LiveKinds, kind) {
				t.Errorf("%s deletes live kind %q", c.name, kind)
			}
		}
	}
}

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
	if v.obsoleteDone != nil {
		<-v.obsoleteDone
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
		if state.GetAdoption() != block_gc.Adoption_ADOPTION_UNADOPTED || len(state.GetObsoleteKinds()) != 0 || state.GetGeneration() != block_gc.Generation_GENERATION_OPEN {
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
	setGeneration(t, v2, block_gc.Generation_GENERATION_SETTLED)
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
	// naming C with a fill mark and no holder for C, opens unadopted, its
	// adoption counts C under B, and the obsolete sweep deletes the mark.
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
	if v3.obsoleteDone != nil {
		<-v3.obsoleteDone
	}
	if n := countPrefix(t, store3, []byte{livenessPrefix[0], 'F'}); n != 0 {
		t.Fatalf("%d fill marks remain after the obsolete sweep", n)
	}
	clk3.Add(block_gc.DefaultGrace + time.Second)
	collectAll(t, v3)
	requireStored(t, v3, tr3, true, "B", "C")
	requireAudit(t, v3, 0)

	// An adopted format 9 store that owes releases, P's through a cascade
	// record and R2's through a paged release, opens unadopted with its
	// release dropped, its old kinds obsolete and its pop mark moved. The
	// adoption recounts from the lists, so neither owed release moves C
	// again, and the collection frees the dead P, G and H2 and keeps every
	// block a root reaches, whatever the stale queue entries name.
	t.Run("format 9 with owed releases", func(t *testing.T) {
		store := store_kvtx_inmem.NewStore()
		tr := format9Owed(t)
		writeFormat9Owed(t, store, keys, tr)
		v, clk := openLivenessVolume(t, store, true)
		state := waitAdopted(t, v)
		if state.GetRelease() != nil {
			t.Fatalf("the adopted state keeps the release %v", state.GetRelease())
		}
		if v.obsoleteDone != nil {
			<-v.obsoleteDone
		}
		for _, kind := range slices.Concat(block_gc.Format9Kinds, []byte{'p', 'D'}) {
			if n := countPrefix(t, store, []byte{livenessPrefix[0], kind}); n != 0 {
				t.Fatalf("%d format 9 records of kind %q remain", n, kind)
			}
		}
		if holders, _ := blockHolders(t, v, tr, "C"); holders != 2 {
			t.Fatalf("C has %d holders after the recount, want B's and P's", holders)
		}
		collectSettled(t, v, clk)
		collectSettled(t, v, clk)
		requireStored(t, v, tr, true, "A", "B", "C", "S")
		requireStored(t, v, tr, false, "P", "G", "H2")
		if holders, _ := blockHolders(t, v, tr, "C"); holders != 1 {
			t.Fatalf("C has %d holders after P's free, want B's", holders)
		}
		requireAudit(t, v, 0)
	})
}

// format9Owed builds the blocks of a format 9 store that owes releases: main
// holds A -> B -> C and s holds S; P -> {C, G} reached zero and owes its
// children their release through a cascade record; R2 -> {C, H2} was freed
// and its paged release moved C but owes H2. It returns the tree; R2's bytes
// and list are gone.
func format9Owed(t *testing.T) testTree {
	t.Helper()
	tr := testTree{}
	tr.add(t, "C")
	tr.add(t, "B", "C")
	tr.add(t, "A", "B")
	tr.add(t, "S")
	tr.add(t, "G")
	tr.add(t, "P", "C", "G")
	tr.add(t, "H2")
	tr.add(t, "R2", "C", "H2")
	return tr
}

// writeFormat9Owed writes the adopted format 9 store of format9Owed's tr: the
// blocks but R2, their lists, the roots main on A and s on S, format 9's
// counts with P's zero record in its uvarint encoding, P's cascade record
// owing its release, the paged release of R2 at its second child, stale queue
// entries naming the live S and C, a journal record and the pop mark.
func writeFormat9Owed(t *testing.T, store kvtx.Store, keys *store_kvkey.KVKey, tr testTree) {
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
	raw := func(kind byte, parts ...[]byte) []byte {
		return slices.Concat(append([][]byte{{livenessPrefix[0], kind}}, parts...)...)
	}
	be := func(n uint64) []byte { return binary.BigEndian.AppendUint64(nil, n) }

	// Write the blocks but the freed R2, their lists and the roots.
	rec := block_gc.NewRecords(tx, livenessPrefix)
	for name, entry := range tr {
		if name == "R2" {
			continue
		}
		key, err := entry.Ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		set(keys.GetBlockKey(key), entry.Data)
		if err := rec.PutChildren(ctx, entry.Ref, entry.Refs); err != nil {
			t.Fatal(err)
		}
	}
	for name, root := range map[string]string{"main": "A", "s": "S"} {
		if err := rec.SetRoot(ctx, "b", name, &block_gc.RootEntry{Ref: block_gc.BlockKey(tr[root].Ref)}); err != nil {
			t.Fatal(err)
		}
	}

	// Write format 9's counts: C counts B and the Dying P, R2's release
	// having moved it, P is at zero at ripe 7 in format 9's encoding, and
	// every other block has its one implied holder.
	if err := rec.SetHolders(ctx, tr["C"].Ref, 2, 0); err != nil {
		t.Fatal(err)
	}
	set(raw('H', block_gc.BlockKey(tr["P"].Ref)), binary.AppendUvarint([]byte{0}, 7))
	set(raw('C', block_gc.BlockKey(tr["P"].Ref)), []byte{'d'})

	// Write the release of R2 owing H2, the stale queue entries, a journal
	// record and the pop mark.
	set(raw('D', be(0)), block_gc.EncodeChildren([]*block.BlockRef{tr["C"].Ref, tr["H2"].Ref}))
	set(raw('Q', be(3), block_gc.BlockKey(tr["S"].Ref)), nil)
	set(raw('Q', be(2), block_gc.BlockKey(tr["C"].Ref)), nil)
	set(raw('J', be(1)), []byte{0x08, 0x01})
	set(raw('p'), be(9))
	state := &block_gc.LivenessState{
		Adoption:   block_gc.Adoption_ADOPTION_ADOPTED,
		Generation: block_gc.Generation_GENERATION_OPEN,
		Frees:      true,
		Release:    &block_gc.Release{Block: block_gc.BlockKey(tr["R2"].Ref), Children: 2, Next: 1},
	}
	if err := rec.SetState(ctx, state); err != nil {
		t.Fatal(err)
	}
	set(keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(9))
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// collectSettled lets the grace period pass and collects until the
// generation is Settled, failing after 16 passes.
func collectSettled(t *testing.T, v *Volume, clk *testClock) {
	t.Helper()
	for range 16 {
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		if livenessGeneration(t, v) == block_gc.Generation_GENERATION_SETTLED {
			return
		}
	}
	t.Fatalf("collection never settled; the generation is %v", livenessGeneration(t, v))
}

// writeFormat7Fill writes an adopted format 7 store holding the blocks of tr
// with their child lists, a named root main of bucket b on root, and a fill
// mark on root, as a format 7 fill left a block whose children it gave no
// holder.
func writeFormat7Fill(t *testing.T, store kvtx.Store, keys *store_kvkey.KVKey, tr testTree, root *block.BlockRef) {
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
func writeLegacyStore(t *testing.T, store kvtx.Store, keys *store_kvkey.KVKey, tr testTree, root string, lists bool, listed ...string) {
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
func countPrefix(t *testing.T, store kvtx.Store, prefix []byte) int {
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

// blockHolders returns the netted holders and ripe sequence of the block tr
// names name.
func blockHolders(t *testing.T, v *Volume, tr testTree, name string) (uint64, uint64) {
	t.Helper()
	var holders, ripe uint64
	err := v.readRecords(t.Context(), func(rec *block_gc.Records) error {
		var err error
		holders, ripe, err = rec.GetHolders(t.Context(), tr[name].Ref)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return holders, ripe
}

// testAuditWalk runs the streaming audit while a writer moves roots onto new
// trees and collects with frees on. main -> R -> H, and the store loses H
// with its list: a hole a held root reaches. main2 -> R2 -> H2, whose hole a
// stage refilled before any audit recorded it, so H2 is undercounted. A stage
// stores P ahead of its child C, which lands while the walk waits out its
// confirm delay. X is named by more roots than one view of a walk reads.
// The walk reports H as unconfirmed and held, H2 as the one undercount, and
// C as settled, and then refuses its scratch store to a second run. A walk
// of the quiet Volume sums X's namers across its views, finds H2 alone
// undercounted, and counts no block wide.
func testAuditWalk(t *testing.T) {
	// Build the hole, the undercount and the parent ahead of its child.
	v, clk := newLivenessVolume(t, true)
	swapCollector(v, v.newCollector(collectorStore{v: v}, true, 0))
	ctx := t.Context()
	tr := testTree{}
	h := tr.add(t, "H")
	tr.add(t, "R", "H")
	tr.add(t, "H2")
	tr.add(t, "R2", "H2")
	tr.add(t, "C")
	tr.add(t, "P", "C")
	tr.add(t, "X")
	roots := []block.NamedRoot{{Name: "main", Ref: tr["R"].Ref}, {Name: "main2", Ref: tr["R2"].Ref}}
	for i := range 4100 {
		roots = append(roots, block.NamedRoot{Name: "wide" + strconv.Itoa(i), Ref: tr["X"].Ref})
	}
	if err := v.SetBucketRoots(ctx, "b", tr.entries("H", "R", "H2", "R2", "X"), roots); err != nil {
		t.Fatal(err)
	}
	punchHole(t, v, h)
	punchHole(t, v, tr["H2"].Ref)
	refill, release, err := v.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.PrepareStagedBlockBatch(ctx, refill, tr.entries("H2")); err != nil {
		t.Fatal(err)
	}
	release()
	stage, _, err := v.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("P")); err != nil {
		t.Fatal(err)
	}

	// Start the walk once the writer has written a root of each name and
	// more than a check's copy of up log entries, so the walk reads the roots
	// in two views with writes between, and its first check sums up log
	// entries past its copy.
	stop, started, errc := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() { errc <- auditWalkWriter(ctx, v, stop, started) }()
	<-started

	// Walk, landing C while the walk waits out its delay.
	scratch, err := store_kvtx_sqlite.Open(ctx, filepath.Join(t.TempDir(), "walk.db"), "walk")
	if err != nil {
		t.Fatal(err)
	}
	defer scratch.Close()
	wait := func(ctx context.Context, d time.Duration) error {
		if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("C")); err != nil {
			return err
		}
		clk.Add(d)
		return nil
	}
	res, err := v.AuditWalk(ctx, scratch, block_gc.WalkOptions{Run: "first", Wait: wait})
	close(stop)
	if werr := <-errc; werr != nil {
		t.Fatal(werr)
	}
	if err != nil {
		t.Fatal(err)
	}

	// Check the report.
	c := res.Counts
	if c.GetUndercount() != 1 || !slices.ContainsFunc(res.UndercountSample, tr["H2"].Ref.EqualsRef) {
		t.Fatalf("walk found %d undercounts, sample %v, want H2 alone", c.GetUndercount(), res.UndercountSample)
	}
	if c.GetCandidates() != 2 || c.GetSettled() != 1 || c.GetUnconfirmed() != 1 || c.GetUnconfirmedHeld() != 1 {
		t.Fatalf("walk counted %d candidates, %d settled, %d unconfirmed, %d of them held, want 2, 1, 1 and 1",
			c.GetCandidates(), c.GetSettled(), c.GetUnconfirmed(), c.GetUnconfirmedHeld())
	}
	if len(res.UnconfirmedSample) != 1 || !res.UnconfirmedSample[0].Ref.EqualsRef(h) {
		t.Fatalf("walk sampled %v unconfirmed, want H", res.UnconfirmedSample)
	}

	// A second run refuses the first run's scratch store.
	if _, err := v.AuditWalk(ctx, scratch, block_gc.WalkOptions{Run: "second"}); !errors.Is(err, block_gc.ErrWalkTaken) {
		t.Fatalf("second run on the scratch store returned %v, want ErrWalkTaken", err)
	}

	// A walk of the quiet Volume checks the wide X soundly.
	quiet, err := store_kvtx_sqlite.Open(ctx, filepath.Join(t.TempDir(), "quiet.db"), "walk")
	if err != nil {
		t.Fatal(err)
	}
	defer quiet.Close()
	res, err = v.AuditWalk(ctx, quiet, block_gc.WalkOptions{Run: "quiet", Delay: -1})
	if err != nil {
		t.Fatal(err)
	}
	if c := res.Counts; c.GetUndercount() != 1 || c.GetWide() != 0 || !slices.ContainsFunc(res.UndercountSample, tr["H2"].Ref.EqualsRef) {
		t.Fatalf("quiet walk found %d undercounts, sample %v, and %d wide, want H2 alone and none", c.GetUndercount(), res.UndercountSample, c.GetWide())
	}

	// Between X's two views, a recount moves one root off X and another onto
	// it with no log entry. Its first view counts the edges of wide0
	// to wide995 in name order, the second those of wide996 to wide999.
	// wide999 leaves X before the first view; while the Volume is unadopted,
	// wide0 leaves and wide999 returns; the recount writes X's holders as
	// the first view read them. The mark must move, so X is wide and not
	// undercounted.
	tr.add(t, "Y")
	cycled := &walkHookStore{collectorStore: collectorStore{v: v}}
	cycled.hook = func(prog *block_gc.WalkProgress) error {
		if prog.GetPhase() != block_gc.WalkPhase_WALK_PHASE_CHECK {
			return nil
		}
		switch cycled.step {
		case 0:
			// Move wide999 to Y and net it.
			cycled.step++
			if err := v.SetBucketRoots(ctx, "b", tr.entries("Y"), []block.NamedRoot{{Name: "wide999", Ref: tr["Y"].Ref, Previous: tr["X"].Ref}}); err != nil {
				return err
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
		case 1:
			// Past X's first view, swap wide0 and wide999 unadopted, then
			// count again.
			if prog.GetWideSeen() == 0 {
				return nil
			}
			cycled.step++
			frees := livenessState(t, v).GetFrees()
			for _, on := range []bool{!frees, frees} {
				if _, err := block_gc.SetFrees(ctx, collectorStore{v: v}, on); err != nil {
					return err
				}
			}
			roots := []block.NamedRoot{
				{Name: "wide0", Ref: tr["Y"].Ref, Previous: tr["X"].Ref},
				{Name: "wide999", Ref: tr["X"].Ref, Previous: tr["Y"].Ref},
			}
			if err := v.SetBucketRoots(ctx, "b", nil, roots); err != nil {
				return err
			}
			_, err := v.Adopt(ctx)
			return err
		}
		return nil
	}
	cycledScratch, err := store_kvtx_sqlite.Open(ctx, filepath.Join(t.TempDir(), "cycled.db"), "walk")
	if err != nil {
		t.Fatal(err)
	}
	defer cycledScratch.Close()
	cycled.scratch = cycledScratch
	res, err = block_gc.Walk(ctx, cycled, cycledScratch, block_gc.WalkOptions{Run: "cycled", Delay: -1, Now: v.now})
	if err != nil {
		t.Fatal(err)
	}
	if c := res.Counts; cycled.step != 2 || c.GetWide() != 1 || slices.ContainsFunc(res.UndercountSample, tr["X"].Ref.EqualsRef) {
		t.Fatalf("cycled walk took %d steps, found %d wide and undercount sample %v, want 2, X wide and not undercounted", cycled.step, c.GetWide(), res.UndercountSample)
	}
}

// testAuditWalkStalled names one block S from about 76,000 parents while
// netting stalls, so the up log past the cursor holds more entries of S than
// one check view may read. The walk copies them all into scratch, and every
// view of S then finds more copies than its budget: the walk must stay
// behind rather than read them all in one view.
func testAuditWalkStalled(t *testing.T) {
	// Land S, then batches of 2,000 parents of S under one root each. Every
	// batch after the first logs an up of S per parent.
	v, _ := newLivenessVolume(t, true)
	ctx := t.Context()
	s := []byte("stalled shared leaf")
	sRef, err := block.BuildBlockRef(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	for b := range 38 {
		entries := []*block.PutBatchEntry{{Ref: sRef, Data: s, RefsKnown: true}}
		var parents []*block.BlockRef
		for i := range 2000 {
			p := []byte("stalled parent " + strconv.Itoa(b) + " " + strconv.Itoa(i))
			pRef, err := block.BuildBlockRef(p, nil)
			if err != nil {
				t.Fatal(err)
			}
			entries = append(entries, &block.PutBatchEntry{Ref: pRef, Data: p, Refs: []*block.BlockRef{sRef}, RefsKnown: true})
			parents = append(parents, pRef)
		}
		g := []byte("stalled grandparent " + strconv.Itoa(b))
		gRef, err := block.BuildBlockRef(g, nil)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, &block.PutBatchEntry{Ref: gRef, Data: g, Refs: parents, RefsKnown: true})
		if err := v.SetBucketRoots(ctx, "b", entries, []block.NamedRoot{{Name: "g" + strconv.Itoa(b), Ref: gRef}}); err != nil {
			t.Fatal(err)
		}
	}

	// Walk without netting. The copy of the up log ends within 20 views, so
	// a walk still behind at its 40th wait reads S's copies against the
	// budget; stop it there.
	scratch, err := store_kvtx_sqlite.Open(ctx, filepath.Join(t.TempDir(), "stalled.db"), "walk")
	if err != nil {
		t.Fatal(err)
	}
	defer scratch.Close()
	errStalled := errors.New("still behind")
	var waits int
	wait := func(context.Context, time.Duration) error {
		if waits++; waits == 40 {
			return errStalled
		}
		return nil
	}
	res, err := v.AuditWalk(ctx, scratch, block_gc.WalkOptions{Run: "stalled", Delay: -1, Wait: wait})
	if !errors.Is(err, errStalled) {
		t.Fatalf("walk under stalled netting returned %v with %+v after %d waits, want it behind at 40", err, res, waits)
	}
}

// walkHookStore is a walk's view of a Volume that calls hook with the walk's
// progress in scratch before each view.
type walkHookStore struct {
	collectorStore
	// scratch is the walk's scratch store.
	scratch kvtx.Store
	// hook runs before each view.
	hook func(prog *block_gc.WalkProgress) error
	// step is how far the hook has gone.
	step int
}

// View reads the walk's progress, runs the hook, then runs fn in a view.
func (s *walkHookStore) View(ctx context.Context, fn func(rec *block_gc.Records) error) error {
	tx, err := s.scratch.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	value, found, err := tx.Get(ctx, []byte{'p'})
	tx.Discard()
	if err != nil {
		return err
	}
	prog := &block_gc.WalkProgress{}
	if found {
		if err := prog.UnmarshalVT(value); err != nil {
			return err
		}
	}
	if err := s.hook(prog); err != nil {
		return err
	}
	return s.collectorStore.View(ctx, fn)
}

// auditWalkWrites is how many roots auditWalkWriter writes: two up log
// entries each past the first 300, so the 2500 before the walk starts leave
// more than walkSliceEdges entries of up log for the walk's first check.
const auditWalkWrites = 3000

// auditWalkWriter moves roots onto new three-block trees and runs a
// collection pass after each. It cycles through 300 root names, more than
// one view of a walk reads, closes started 500 writes before its last, and
// returns once it has written auditWalkWrites roots and stop closes.
func auditWalkWriter(ctx context.Context, v *Volume, stop, started chan struct{}) error {
	prev := make([]*block.BlockRef, 300)
	for i := 0; ; i++ {
		// Stop when asked.
		select {
		case <-stop:
			return nil
		default:
		}
		switch i {
		case auditWalkWrites - 500:
			close(started)
		case auditWalkWrites:
			<-stop
			return nil
		}

		// Write a leaf, one of eight shared leaves and their parent under the
		// next root name, and collect. Each parent adds an up log entry to
		// its shared leaf, so the check of every slice sums pairs the walk
		// copied in earlier slices and pairs past its last copy.
		leaf := []byte("writer leaf " + strconv.Itoa(i))
		leafRef, err := block.BuildBlockRef(leaf, nil)
		if err != nil {
			return err
		}
		shared := []byte("writer shared " + strconv.Itoa(i%8))
		sharedRef, err := block.BuildBlockRef(shared, nil)
		if err != nil {
			return err
		}
		parent := append([]byte("writer parent "), leafRef.MarshalString()...)
		parentRef, err := block.BuildBlockRef(parent, nil)
		if err != nil {
			return err
		}
		entries := []*block.PutBatchEntry{
			{Ref: leafRef, Data: leaf, RefsKnown: true},
			{Ref: sharedRef, Data: shared, RefsKnown: true},
			{Ref: parentRef, Data: parent, Refs: []*block.BlockRef{leafRef, sharedRef}, RefsKnown: true},
		}
		name := i % 300
		root := []block.NamedRoot{{Name: "w" + strconv.Itoa(name), Ref: parentRef, Previous: prev[name]}}
		if err := v.SetBucketRoots(ctx, "w", entries, root); err != nil {
			return err
		}
		prev[name] = parentRef
		if _, err := v.Collect(ctx); err != nil {
			return err
		}
	}
}

// BenchmarkLandCommit times one World-shaped commit on s4db: a new parent
// naming fanout stored leaves, put with the move of its root, so its Land
// checks fanout outside children, as a commit that rewrites a tree page
// does. It reports the store reads each commit makes, a batched read
// counting once.
func BenchmarkLandCommit(b *testing.B) {
	const fanout = 250

	// Open the Volume over a counting s4db store, frees off as on the Mac.
	ctx := b.Context()
	db, err := s4db.Open(filepath.Join(b.TempDir(), "bench.s4db"), s4db.Options{})
	if err != nil {
		b.Fatal(err)
	}
	cs := kvtx_kvtest.NewCountingStore(db)
	v, err := NewVolume(ctx, "test/land-commit", store_kvkey.NewDefaultKVKey(), cs, &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		_ = v.Close()
		_ = db.Close()
	})
	if v.adoptDone != nil {
		<-v.adoptDone
	}

	// Store the leaves under one root.
	entry := func(data []byte, refs []*block.BlockRef) *block.PutBatchEntry {
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			b.Fatal(err)
		}
		return &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}
	}
	var leaves []*block.PutBatchEntry
	var refs []*block.BlockRef
	for i := range fanout {
		e := entry([]byte("leaf "+strconv.Itoa(i)), nil)
		leaves, refs = append(leaves, e), append(refs, e.Ref)
	}
	base := entry([]byte("base"), refs)
	if err := v.SetBucketRoots(ctx, "b", append(leaves, base), []block.NamedRoot{{Name: "base", Ref: base.Ref}}); err != nil {
		b.Fatal(err)
	}

	// Commit a new parent of the leaves under main each iteration.
	var prev *block.BlockRef
	start := cs.Counts().Reads
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		p := entry([]byte("parent "+strconv.Itoa(i)), refs)
		if err := v.SetBucketRoots(ctx, "b", []*block.PutBatchEntry{p}, []block.NamedRoot{{Name: "main", Ref: p.Ref, Previous: prev}}); err != nil {
			b.Fatal(err)
		}
		prev = p.Ref
	}
	b.StopTimer()
	b.ReportMetric(float64(cs.Counts().Reads-start)/float64(b.N), "reads/op")
}

// sliceStore is an in-memory store that counts the write transactions while
// armed and runs hook before each after the first, so a test acts between the
// slices of a staged batch.
type sliceStore struct {
	*store_kvtx_inmem.Store
	// armed counts the write transactions while set.
	armed bool
	// opened counts the write transactions since the store was armed.
	opened int
	// hook runs before the write after the n-th, with no store writer held.
	// The writes it makes are not counted.
	hook func(n int)
}

// NewTransaction runs hook before an armed write after the first, then opens
// the transaction, refusing it when hook cancelled ctx.
func (s *sliceStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write && s.armed {
		s.opened++
		if s.opened > 1 {
			s.armed = false
			s.hook(s.opened - 1)
			s.armed = true
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
	}
	return s.Store.NewTransaction(ctx, write)
}

// sliceMode is what testStageSlices does with a staged batch.
type sliceMode int

const (
	// sliceCollect collects past the grace period between slices.
	sliceCollect sliceMode = iota
	// sliceCancelRetry cancels after the first slice, then retries the batch.
	sliceCancelRetry
	// sliceCancelRelease cancels after the first slice, then releases the
	// stage.
	sliceCancelRelease
	// sliceRelease releases the stage after the first slice.
	sliceRelease
	// sliceParentFirst orders the parent before its children.
	sliceParentFirst
	// sliceOneTransaction opens the Volume with Config.StageOneTransaction.
	sliceOneTransaction
)

// testStageSlices lands a stage's batch of seven leaves and their parent in
// slices of three, so the parent lands two transactions after its first
// children, and between slices runs a collector pass past the grace period,
// cancels the batch or releases the stage. No block of a landed slice may go
// while the stage holds it; a retry lands the rest and finds the earlier
// slices stored; a release refuses the next slice and, like a cancellation
// without a retry, leaves the landed blocks to the collector. A batch ordered
// parent first, or under StageOneTransaction, lands in one transaction.
func testStageSlices(t *testing.T, mode sliceMode) {
	store := &sliceStore{Store: store_kvtx_inmem.NewStore()}
	v, clk := openConfVolume(t, store, &store_kvtx.Config{
		CollectorFrees:      true,
		StageOneTransaction: mode == sliceOneTransaction,
	})
	if mode != sliceOneTransaction {
		v.stageSlice = 3
	}
	stage, release, err := v.OpenStage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	tr := testTree{}
	leaves := []string{"C0", "C1", "C2", "C3", "C4", "C5", "C6"}
	for _, name := range leaves {
		tr.add(t, name)
	}
	p := tr.add(t, "P", leaves...)
	names := append(append([]string(nil), leaves...), "P")
	order := names
	if mode == sliceParentFirst {
		order = append([]string{"P"}, leaves...)
	}

	// Between slices, collect past the grace period and check that every
	// landed block stays, or cancel or release after the first slice.
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	store.hook = func(n int) {
		switch mode {
		case sliceCancelRetry, sliceCancelRelease:
			stop()
		case sliceRelease:
			release()
		default:
			advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
			collectAll(t, v)
			requireStored(t, v, tr, true, names[:3*n]...)
		}
	}
	store.armed = true
	_, err = v.PrepareStagedBlockBatch(ctx, stage, tr.entries(order...))
	store.armed = false
	testStageSliceResult(t, mode, store.opened, err)

	// A cancelled batch leaves the first slice held by the stage, and a retry
	// finds it. A release while the second slice is open is carried by that
	// slice's transaction, and refuses the third.
	if mode == sliceCancelRetry || mode == sliceCancelRelease {
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, names[:3]...)
		requireStored(t, v, tr, false, names[3:]...)
	}
	if mode == sliceCancelRetry {
		existed, err := v.PrepareStagedBlockBatch(t.Context(), stage, tr.entries(names...))
		if err != nil {
			t.Fatal(err)
		}
		for i, got := range existed {
			if got != (i < 3) {
				t.Fatalf("retry found entry %d existed=%v, want %v", i, got, i < 3)
			}
		}
	}
	if mode == sliceCancelRelease || mode == sliceRelease {
		// The release, applied by the next direct transaction, leaves the
		// landed leaves to the collector.
		release()
		if err := v.withDirectAtomic(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, names...)
		requireAudit(t, v, 0)
		return
	}

	// Main adopts the parent, the stage lets go, and every block stays.
	if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
		t.Fatal(err)
	}
	release()
	clk.Add(block_gc.DefaultGrace + time.Second)
	collectAll(t, v)
	requireStored(t, v, tr, true, names...)
	requireAudit(t, v, 0)
}

// testStageSliceResult checks the refusal and the write transactions of
// testStageSlices' batch under mode.
func testStageSliceResult(t *testing.T, mode sliceMode, writes int, err error) {
	t.Helper()
	want, wantErr := 3, error(nil)
	switch mode {
	case sliceCancelRetry, sliceCancelRelease:
		want, wantErr = 2, context.Canceled
	case sliceRelease:
		want, wantErr = 2, block.ErrStageReleased
	case sliceParentFirst, sliceOneTransaction:
		want = 1
	}
	if !errors.Is(err, wantErr) || (wantErr == nil && err != nil) {
		t.Fatalf("batch returned %v, want %v", err, wantErr)
	}
	if writes != want {
		t.Fatalf("batch opened %d write transactions, want %d", writes, want)
	}
}
