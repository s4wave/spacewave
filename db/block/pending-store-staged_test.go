package block_test

import (
	"context"
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
