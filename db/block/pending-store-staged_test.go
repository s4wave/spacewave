package block_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// sweepingStore is an inner store that holds roots and runs sweep as a stage
// opens, which a PendingStore does after it keeps a batch and before it
// stores the entries past its bound.
type sweepingStore struct {
	block.StoreOps
	sweep func()
}

// HoldsRoots reports that the store holds roots.
func (s *sweepingStore) HoldsRoots() bool { return true }

// PinRoot holds nothing.
func (s *sweepingStore) PinRoot(context.Context, *block.BlockRef) (func(), error) {
	return func() {}, nil
}

// OpenStage runs the sweep and returns the store itself.
func (s *sweepingStore) OpenStage(context.Context) (block.StoreOps, func(), error) {
	if s.sweep != nil {
		s.sweep()
	}
	return s.StoreOps, func() {}, nil
}

// ReleaseRoots releases nothing.
func (s *sweepingStore) ReleaseRoots(context.Context, []*block.BlockRef) error { return nil }

// TestPutBlockBatchStagedHoldsKeptChildren checks that a sweep between
// PutBlockBatchStaged keeping a child and storing the parent past the bound
// that names it cannot drop the child: the child still reads between the
// sweep and the store, and the stored parent never names a block neither kept
// nor stored. The sweep's live root is the parent, as an admitted candidate's
// would be, and it reaches no kept block.
func TestPutBlockBatchStagedHoldsKeptChildren(t *testing.T) {
	ctx := context.Background()
	inner := &sweepingStore{StoreOps: block_mock.NewMockStore(0)}
	pages := block.NewPendingStore(inner, 1<<10)
	entry := func(data []byte, refs ...*block.BlockRef) *block.PutBatchEntry {
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		return &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}
	}
	child := entry([]byte("kept child"))
	parent := entry(make([]byte, 2<<10), child.Ref)
	var readErr error
	inner.sweep = func() {
		pages.Sweep([]*block.BlockRef{parent.Ref}, pages.SweepMark())
		if _, found, err := pages.GetBlock(ctx, child.Ref); err != nil || !found {
			readErr = errors.Errorf("the kept child read found=%v err=%v between the sweep and the store", found, err)
		}
	}

	release, err := pages.PutBlockBatchStaged(ctx, []*block.PutBatchEntry{child, parent})
	if err != nil {
		t.Fatal(err)
	}
	if release == nil {
		t.Fatal("the parent past the bound opened no stage")
	}
	release()
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, ref := range []*block.BlockRef{parent.Ref, child.Ref} {
		found, err := inner.GetBlockExists(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("block %s is not stored after the put", ref.MarshalString())
		}
	}
}

// tieredStore is a sweepingStore with a local tier: the whole store's
// existence answer is remote's, and it counts the calls to each tier.
type tieredStore struct {
	*sweepingStore
	local       *countingExists
	remote      func(ctx context.Context, refs []*block.BlockRef) ([]bool, error)
	remoteCalls int
}

// countingExists counts the existence reads of its store.
type countingExists struct {
	block.StoreOps
	calls int
}

// GetBlockExistsBatch counts the read and forwards it.
func (c *countingExists) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	c.calls++
	return c.StoreOps.GetBlockExistsBatch(ctx, refs)
}

// GetLocalStore returns the local tier.
func (s *tieredStore) GetLocalStore() block.StoreOps { return s.local }

// GetBlockExistsBatch answers for the whole store.
func (s *tieredStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.remoteCalls++
	return s.remote(ctx, refs)
}

// TestChildCheckRefusesOnlyOnProof checks the child check of a store with a
// local tier: a child the local tier holds passes with one local read and no
// call behind it; a child the whole store could not look for, offline or with
// no peer session, or slow past the probe's bound, is admitted and counted
// unchecked; a child no source holds, as after the local collector freed it,
// is refused.
func TestChildCheckRefusesOnlyOnProof(t *testing.T) {
	ctx := context.Background()
	mock := block_mock.NewMockStore(0)
	inner := &tieredStore{sweepingStore: &sweepingStore{StoreOps: mock}, local: &countingExists{StoreOps: mock}}
	pages := block.NewPendingStore(inner, 1<<20)
	var reported int
	pages.SetUncheckedReport(func(string, int, error) { reported++ })
	flush := func(name string, child *block.BlockRef) error {
		data := []byte("parent " + name)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		entry := &block.PutBatchEntry{Ref: ref, Data: data, Refs: []*block.BlockRef{child}, RefsKnown: true}
		if _, err := pages.PutBlockBatch(ctx, []*block.PutBatchEntry{entry}); err != nil {
			t.Fatal(err)
		}
		release, err := pages.Flush(ctx, []*block.BlockRef{ref})
		if release != nil {
			release()
		}
		return err
	}
	child := func(name string) *block.BlockRef {
		ref, err := block.BuildBlockRef([]byte("child "+name), nil)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}

	// A local child: one local read, nothing behind it.
	local, _, err := mock.PutBlock(ctx, []byte("child local"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := flush("local", local); err != nil || inner.local.calls != 1 || inner.remoteCalls != 0 {
		t.Fatalf("local child: %v, local reads %d, remote reads %d", err, inner.local.calls, inner.remoteCalls)
	}

	// Offline or with no peer session: admitted, counted and reported.
	inner.remote = func(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
		if block.GetPeerWait(ctx) == nil {
			// A store read that may wait for a peer answers a miss once its
			// sources are away, so a probe without the marker refuses here.
			return make([]bool, len(refs)), nil
		}
		return nil, errors.Wrap(block.ErrUnavailable, "storage backend not open")
	}
	if err := flush("offline", child("offline")); err != nil || pages.Unchecked() != 1 || reported != 1 {
		t.Fatalf("unknown child: %v, unchecked %d, reported %d", err, pages.Unchecked(), reported)
	}

	// A source that never answers: the write waits only the probe's bound,
	// then is admitted, counted and reported.
	inner.remote = func(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
		<-ctx.Done()
		return nil, context.Cause(ctx)
	}
	start := time.Now()
	if err := flush("slow", child("slow")); err != nil || pages.Unchecked() != 2 || reported != 2 {
		t.Fatalf("slow child: %v, unchecked %d, reported %d", err, pages.Unchecked(), reported)
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Fatalf("slow child: the write waited %v on the probe", waited)
	}

	// Held behind the local tier: admitted, not counted.
	inner.remote = func(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
		return []bool{true}, nil
	}
	if err := flush("remote", child("remote")); err != nil || pages.Unchecked() != 2 {
		t.Fatalf("remote child: %v, unchecked %d", err, pages.Unchecked())
	}

	// Absent everywhere: refused.
	inner.remote = func(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
		return []bool{false}, nil
	}
	if err := flush("freed", child("freed")); !errors.Is(err, block.ErrMissingChild) {
		t.Fatalf("freed child: %v, want %v", err, block.ErrMissingChild)
	}
}

// existsStore is a store whose existence lookups answer with exists, and
// whose local tier, when set, is local.
type existsStore struct {
	*sweepingStore
	exists func(ctx context.Context, ref *block.BlockRef) (bool, error)
	local  block.StoreOps
}

// GetBlockExists answers with exists.
func (s *existsStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return s.exists(ctx, ref)
}

// GetLocalStore returns the local tier.
func (s *existsStore) GetLocalStore() block.StoreOps {
	return s.local
}

// TestFlushPinProbeSkipsOnlyUnknown checks Flush's lookup of a root it does
// not keep, as the child check makes it: a local tier error fails the flush,
// while a whole-store lookup that errs, finds no peer session or runs past
// the probe's bound is unknown, so the pin is skipped, the root is recorded
// for RunPins and the flush lands, and a parent cancel during it fails the flush. It fails when a fast remote
// error fails a write, or a local one is taken for a remote one.
func TestFlushPinProbeSkipsOnlyUnknown(t *testing.T) {
	root, err := block.BuildBlockRef([]byte("unkept root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	local := &existsStore{sweepingStore: &sweepingStore{StoreOps: block_mock.NewMockStore(0)}, exists: func(context.Context, *block.BlockRef) (bool, error) {
		return false, nil
	}}
	whole := &existsStore{sweepingStore: &sweepingStore{StoreOps: block_mock.NewMockStore(0)}, local: local}
	pages := block.NewPendingStore(whole, 1<<20)
	flush := func(ctx context.Context) error {
		release, err := pages.Flush(ctx, []*block.BlockRef{root})
		if release != nil {
			release()
		}
		return err
	}

	// The whole store's unknown answers skip the pin.
	unknown := map[string]func(context.Context, *block.BlockRef) (bool, error){
		"no peer session": func(context.Context, *block.BlockRef) (bool, error) {
			return false, errors.Wrap(block.ErrUnavailable, "no peer session")
		},
		"remote error": func(context.Context, *block.BlockRef) (bool, error) {
			return false, errors.New("cloud: connection reset")
		},
		"slow": func(ctx context.Context, _ *block.BlockRef) (bool, error) {
			<-ctx.Done()
			return false, ctx.Err()
		},
	}
	for name, exists := range unknown {
		whole.exists = exists
		start := time.Now()
		if err := flush(t.Context()); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if waited := time.Since(start); waited > 5*time.Second {
			t.Fatalf("%s: the flush waited %v on the probe", name, waited)
		}
		if n := pages.Unfetched(); n != 1 {
			t.Fatalf("%s: %d roots recorded for a later pin, want 1", name, n)
		}
	}

	// A parent cancel during the whole-store lookup fails the flush.
	ctx, cancel := context.WithCancel(t.Context())
	whole.exists = func(ctx context.Context, _ *block.BlockRef) (bool, error) {
		cancel()
		<-ctx.Done()
		return false, ctx.Err()
	}
	if err := flush(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("parent cancel: %v, want %v", err, context.Canceled)
	}

	// A local tier error fails the flush.
	broken := errors.New("read error")
	local.exists = func(context.Context, *block.BlockRef) (bool, error) {
		return false, broken
	}
	if err := flush(t.Context()); !errors.Is(err, broken) {
		t.Fatalf("local read error: %v, want %v", err, broken)
	}
}

// healingStore is a whole store whose existence lookups fail with a dial
// error while down, and otherwise find nothing, so a Flush pins the root it
// does not keep. It counts the lookups and pins.
type healingStore struct {
	*sweepingStore
	local  block.StoreOps
	mtx    sync.Mutex
	down   bool
	probes int
	pins   int
}

// GetBlockExists fails while down.
func (s *healingStore) GetBlockExists(context.Context, *block.BlockRef) (bool, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.probes++
	if s.down {
		return false, errors.New("dial tcp: lookup cloud: no such host")
	}
	return false, nil
}

// PinRoot counts the pin.
func (s *healingStore) PinRoot(context.Context, *block.BlockRef) (func(), error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.pins++
	return func() {}, nil
}

// GetLocalStore returns the local tier.
func (s *healingStore) GetLocalStore() block.StoreOps {
	return s.local
}

// set sets down, and returns the lookups and pins so far.
func (s *healingStore) set(down bool) (probes, pins int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.down = down
	return s.probes, s.pins
}

// newHealingPages returns a PendingStore over a whole store that is down,
// with RunPins running until the test ends, and a Flush of root through it.
func newHealingPages(t *testing.T, root *block.BlockRef) (*block.PendingStore, *healingStore, func()) {
	local := &existsStore{sweepingStore: &sweepingStore{StoreOps: block_mock.NewMockStore(0)}, exists: func(context.Context, *block.BlockRef) (bool, error) {
		return false, nil
	}}
	whole := &healingStore{sweepingStore: &sweepingStore{StoreOps: block_mock.NewMockStore(0)}, local: local, down: true}
	pages := block.NewPendingStore(whole, 1<<20)
	go func() { _ = pages.RunPins(t.Context()) }()
	flush := func() {
		release, err := pages.Flush(t.Context(), []*block.BlockRef{root})
		if err != nil {
			t.Fatalf("flush with the store down: %v", err)
		}
		release()
	}
	return pages, whole, flush
}

// waitFor waits up to five seconds for cond.
func waitFor(t *testing.T, what string, cond func() bool) {
	for deadline := time.Now().Add(5 * time.Second); !cond(); {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestFlushRecordsUnfetchedRootAndPinsAfterHeal checks the later pin of a
// root whose Flush met a dial error: the Flush lands without the pin and
// records the root, and once the store answers RunPins pins it and empties
// the set, with no Flush to drive it.
func TestFlushRecordsUnfetchedRootAndPinsAfterHeal(t *testing.T) {
	root, err := block.BuildBlockRef([]byte("unfetched root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	pages, whole, flush := newHealingPages(t, root)
	flush()
	if n := pages.Unfetched(); n != 1 {
		t.Fatalf("%d roots recorded after the failed lookup, want 1", n)
	}
	if _, pins := whole.set(false); pins != 0 {
		t.Fatalf("%d pins taken while the store was down", pins)
	}
	waitFor(t, "the recorded root to be pinned", func() bool { return pages.Unfetched() == 0 })
	if _, pins := whole.set(false); pins != 1 {
		t.Fatalf("%d pins after the heal, want 1", pins)
	}
}

// TestDropUnfetchedSkipsSupersededRoot checks that a recorded root its writer
// no longer names leaves the set with no lookup: after DropUnfetched, the
// store's lookups stop once a pass in flight ends, and the heal pins
// nothing.
func TestDropUnfetchedSkipsSupersededRoot(t *testing.T) {
	root, err := block.BuildBlockRef([]byte("superseded root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	pages, whole, flush := newHealingPages(t, root)
	flush()
	pages.DropUnfetched(nil, pages.SweepMark())
	if n := pages.Unfetched(); n != 0 {
		t.Fatalf("%d roots left after the drop, want 0", n)
	}
	time.Sleep(2 * pinRetryWait)
	probes, _ := whole.set(false)
	time.Sleep(4 * pinRetryWait)
	if after, pins := whole.set(false); after != probes || pins != 0 {
		t.Fatalf("after the drop: %d lookups and %d pins, want none", after-probes, pins)
	}
}

// TestUnfetchedRootStaysWhileStoreDown is adversarial: while every lookup
// fails, the passes keep running and nothing takes a pin or drops the root,
// so the root is never reported held. A mark taken before the record keeps
// it too.
func TestUnfetchedRootStaysWhileStoreDown(t *testing.T) {
	root, err := block.BuildBlockRef([]byte("down root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	pages, whole, flush := newHealingPages(t, root)
	mark := pages.SweepMark()
	flush()
	pages.DropUnfetched(nil, mark)
	first, _ := whole.set(true)
	waitFor(t, "a later pass", func() bool {
		probes, _ := whole.set(true)
		return probes > first+1
	})
	if n := pages.Unfetched(); n != 1 {
		t.Fatalf("%d roots recorded while down, want 1", n)
	}
	if _, pins := whole.set(true); pins != 0 {
		t.Fatalf("%d pins taken while every lookup failed", pins)
	}
}

// pinRetryWait is RunPins' shortest wait between passes.
const pinRetryWait = 250 * time.Millisecond
