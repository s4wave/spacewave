package block_test

import (
	"context"
	"testing"

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
