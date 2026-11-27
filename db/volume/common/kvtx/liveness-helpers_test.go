package kvtx

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"math"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/volume"
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
	return openLivenessVolume(t, newGuardedStore(t), frees)
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

// add builds the block name referencing the blocks selected by children, which
// add built before, and returns its ref.
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

// addBinary builds a tree depth levels deep, each block above the leaves
// naming two, and returns its blocks' names bottom up, the root last.
func (tr testTree) addBinary(t *testing.T, depth int) []string {
	t.Helper()
	var level []string
	for i := range 1 << (depth - 1) {
		name := "level " + strconv.Itoa(depth) + " block " + strconv.Itoa(i)
		tr.add(t, name)
		level = append(level, name)
	}
	names := slices.Clone(level)
	for d := depth - 1; d >= 1; d-- {
		var up []string
		for i := 0; i < len(level); i += 2 {
			name := "level " + strconv.Itoa(d) + " block " + strconv.Itoa(i/2)
			tr.add(t, name, level[i], level[i+1])
			up = append(up, name)
		}
		names = append(names, up...)
		level = up
	}
	return names
}

// entries returns the blocks selected by names.
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

// requireAudit checks that an audit finds no live block queued and the expected
// number of leaked blocks.
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
// not stored: no child is freed while a stored parent lists it, the model's
// NoLiveListsFreed.
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
	// pages selects the up pages from its start up to its end partly netted
	// instead, when its end is set.
	pages [2]uint64
	// until selects the first transaction after which until reports true of
	// the state instead, when set.
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

// viewHook runs a collector's transactions and read snapshots, and calls
// after once, following the first snapshot read once the free's phase
// begins: a writer that commits between the free's read of its candidates
// and its write slice.
type viewHook struct {
	block_gc.Store
	// after runs after the snapshot.
	after func()
	// armed is set once the free's phase began.
	armed bool
	// fired records that after ran.
	fired bool
}

// arm is the collector's phase callback: it arms the hook as the free's
// phase begins.
func (h *viewHook) arm(ph block_gc.Phase) func() {
	if ph == block_gc.PhaseSweep && !h.fired {
		h.armed = true
	}
	return func() {}
}

// View reads the snapshot, then runs after once armed.
func (h *viewHook) View(ctx context.Context, fn func(rec *block_gc.Records) error) error {
	err := h.Store.View(ctx, fn)
	if h.armed && !h.fired {
		h.armed, h.fired = false, true
		h.after()
	}
	return err
}

// errRerun discards the first run of a transaction rerunStore runs twice.
var errRerun = errors.New("transaction discarded to run again")

// rerunStore runs each of a collector's transactions twice: once discarded,
// then again committed, as a store that retries a conflicted transaction.
type rerunStore struct {
	block_gc.Store
}

// Update runs fn in a transaction it discards, then in one it commits.
func (s *rerunStore) Update(ctx context.Context, fn func(rec *block_gc.Records, blocks block.StoreOps) error) error {
	err := s.Store.Update(ctx, func(rec *block_gc.Records, blocks block.StoreOps) error {
		if err := fn(rec, blocks); err != nil {
			return err
		}
		return errRerun
	})
	if !errors.Is(err, errRerun) {
		return err
	}
	return s.Store.Update(ctx, fn)
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

// The transactions of block_gc.Adopt, in order, on a Volume whose records
// each fit in one slice: the claim, the pop mark's raise, the take-out of
// parked and released blocks from the run, the clears of counts, the queue,
// parked blocks, released blocks, the up and down logs, born marks,
// moved-root marks, release pages, recount records and unmarked counts, then
// the mark, the root count, the child list count and the settle.
const (
	// adoptClearCounts clears the count records an earlier run left.
	adoptClearCounts = 5
	// adoptMark marks the Volume adopting.
	adoptMark = 16
	// adoptCountRoots counts the roots.
	adoptCountRoots = 17
	// adoptCountChildren counts the child lists' holders.
	adoptCountChildren = 18
	// adoptSettle settles the stored blocks' counts and marks the Volume
	// adopted.
	adoptSettle = 19
)

// The transactions of a block_gc.Adopt that takes over a run whose cursor is
// at its roots, on a Volume whose records each fit in one slice: the
// take-over, then the root count, the child list count (3) and the settle. A
// pass the cursor has passed still takes its one transaction.
const (
	// resumeCountRoots counts the roots.
	resumeCountRoots = 2
	// resumeSettle settles the stored blocks' counts.
	resumeSettle = 4
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

// Update calls the func at the transaction's position, then runs the
// transaction.
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

// requireFront fails t when, while v's old generation is Open, a block a young
// or verified list references is unverified, held and off the front, as
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

// readOnlyStore is an in-memory store that reports itself read-only, as an s4db
// opened for reading does, so a Volume over it starts no writer. It keeps the
// store's atomic commits, which a Volume needs for liveness.
type readOnlyStore struct {
	*seekGuard
}

// ReadOnly reports that the store refuses every write transaction.
func (readOnlyStore) ReadOnly() bool {
	return true
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

// Update calls before, then runs the transaction.
func (p *walkProbe) Update(ctx context.Context, fn func(rec *block_gc.Records, blocks block.StoreOps) error) error {
	p.before()
	return p.Store.Update(ctx, fn)
}

// copyAll returns an in-memory copy of every key of store.
func copyAll(t *testing.T, store kvtx.Store) kvtx.Store {
	t.Helper()
	ctx := t.Context()
	src, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Discard()
	cp := newGuardedStore(t)
	dst, err := cp.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Discard()
	err = src.ScanPrefix(ctx, nil, func(key, value []byte) error {
		return dst.Set(ctx, bytes.Clone(key), bytes.Clone(value))
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dst.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return cp
}

// adoptedRecords returns the count, queue and front records of store, with
// each zero count's and queued block's ripe sequence dropped: two adoptions
// of the same holders agree on them, while the sequence is the up log's end
// at each settle.
func adoptedRecords(t *testing.T, store kvtx.Store) map[string]string {
	t.Helper()
	out := livenessRecords(t, store, 'H', 'G')
	for key, value := range out {
		if len(value) == 9 && value[0] == 0 {
			out[key] = "zero"
		}
	}
	queueAt := len(livenessPrefix) + 1
	for key := range livenessRecords(t, store, 'Q') {
		out[key[:queueAt]+key[queueAt+8:]] = "queued"
	}
	return out
}

// recordsDiff returns the keys whose values differ between a and b, with
// each side's value.
func recordsDiff(a, b map[string]string) map[string][2]string {
	out := make(map[string][2]string)
	for key, value := range a {
		if b[key] != value {
			out[base64.RawURLEncoding.EncodeToString([]byte(key))] = [2]string{value, b[key]}
		}
	}
	for key, value := range b {
		if _, ok := a[key]; !ok {
			out[base64.RawURLEncoding.EncodeToString([]byte(key))] = [2]string{"", value}
		}
	}
	return out
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

// leaseApart moves the roots of the open stage onto a new lease, standing for
// the lease of another process, and forgets the stage, so this process's
// renewals no longer hold its roots and the lease lapses after rootLease.
func leaseApart(t *testing.T, v *Volume, clk *testClock, stage string) {
	t.Helper()
	ctx := t.Context()

	// Forget the stage, keeping its roots.
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var moves []block.NamedRoot
	lease := ulid.NewULID()
	for name, ref := range v.stages[stage].roots {
		moves = append(moves, block.NamedRoot{Name: name, Ref: ref, Previous: ref, Lease: lease})
	}
	delete(v.stages, stage)
	unlock()

	// Move the roots onto the new lease.
	err = v.withDirectAtomic(ctx, func(p *physicalTx) error {
		if err := p.rec.StartLease(ctx, lease, clk.Now().Add(rootLease).UnixMilli()); err != nil {
			return err
		}
		_, err := v.land(ctx, p, "", nil, moves)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// openStagesTogether opens n stages at once on a fresh Volume, releasing
// each once all are open, and returns the Volume.
func openStagesTogether(t *testing.T, n int) *Volume {
	t.Helper()
	v, _ := newLivenessVolume(t, false)
	ctx := t.Context()

	// Start the openers together.
	var wg sync.WaitGroup
	gate := make(chan struct{})
	errs := make([]error, n)
	releases := make([]func(), n)
	for i := range n {
		wg.Go(func() {
			<-gate
			_, releases[i], errs[i] = v.OpenStage(ctx)
		})
	}
	close(gate)
	wg.Wait()

	// Release the open stages.
	for i, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
		releases[i]()
	}
	return v
}

// requireOneLease fails unless v's store holds exactly one lease record, the
// published process lease.
func requireOneLease(t *testing.T, v *Volume) {
	t.Helper()
	ctx := t.Context()
	var leases []block_gc.LapsedLease
	err := v.readRecords(ctx, func(rec *block_gc.Records) error {
		var err error
		leases, err = rec.LapsedLeases(ctx, "", math.MaxInt64, 16)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(leases) != 1 || leases[0].ID != v.processLeaseID() {
		t.Fatalf("store holds leases %v, want the one process lease %q", leases, v.processLeaseID())
	}
}

// fillLapse is how long after a cache fill its lease has surely lapsed: the
// fill hold, the renewal interval for which one fill lease serves fills, and
// a second.
const fillLapse = block_gc.DefaultGrace + rootRenewInterval + time.Second

// advanceRenewing moves the clock forward by d in steps shorter than the
// lease, renewing this process's leased roots after each, as its renewal
// loop does.
func advanceRenewing(t *testing.T, v *Volume, clk *testClock, d time.Duration) {
	t.Helper()
	for d > 0 {
		step := min(d, rootRenewInterval)
		clk.Add(step)
		d -= step
		if _, err := v.renewRoots(t.Context(), false); err != nil {
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

// admitTree sets root payloads of v onto Q and builds the payload tree P -> L
// and H referencing P and Q, none of them stored. admit moves payloads from Q
// onto H, storing H and the extra blocks and requiring its blocks held.
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

// putFlow returns v's flow for the write path named path.
func putFlow(t *testing.T, v *Volume, path string) volume.PutFlow {
	t.Helper()
	for _, flow := range v.GetFlows().Puts {
		if flow.Path == path {
			return flow
		}
	}
	t.Fatalf("no flow for the path %s", path)
	return volume.PutFlow{}
}

// storeRecords returns every record of store but its format version, by key.
func storeRecords(t *testing.T, store kvtx.Store, keys *store_kvkey.KVKey) map[string][]byte {
	t.Helper()
	tx, err := store.NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	records := make(map[string][]byte)
	version := string(keys.GetFormatVersionKey())
	err = tx.ScanPrefix(t.Context(), nil, func(key, value []byte) error {
		if string(key) != version {
			records[string(key)] = bytes.Clone(value)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return records
}

// runLivenessSettle shares the legacy-store fixtures across settle cases.
func runLivenessSettle(t *testing.T) {
	t.Helper()

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
		store := newGuardedStore(t)
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
			_, _, err := v.landFill(t.Context(), p, "b", tr.entries(names...))
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
	// pairs builds, for each i < n, D<i>, C<i> referencing D<i> and B<i>
	// referencing C<i>, and returns the names to list and the C names to
	// cut.
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
	// passes collects n times, letting the grace period pass before each.
	passes := func(t *testing.T, v *Volume, clk *testClock, n int) {
		t.Helper()
		for range n {
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
		}
	}

	testLivenessSettleBuild(t, adoptOldStore, oldStore, has, fill, rootOnto, verify, requireKept, collectUntil, flipped, pairs, buildRun, oldU, passes, run, front)
	testLivenessSettleSweep(t, oldStore, has, fill, rootOnto, verify, collectUntil, flipped, settled, oldU, run)
	testLivenessSettleFlip(t, oldStore, has, fill, rootOnto, verify, requireKept, collectUntil, flipped, settled, oldU, passes, run, front)
	testLivenessSettleWriters(t, oldStoreOn, verify, flipped, settled, pairs, run)
	testLivenessSettleCopy(t, oldStore, has, fill, rootOnto, verify, collectUntil, flipped, settled, oldU, passes, run, front)
}
