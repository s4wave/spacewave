package block_store_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// HoldingStage is a stage whose volume holds every root it is asked about.
// Only the methods a wrapper forwards in a stage chain are implemented.
type HoldingStage struct {
	block.StoreOps
	block.LeasedRoots
	block.RootProofs

	// held lists the roots held, in call order.
	held []*block.BlockRef
}

// HoldsRoots reports that the stage holds roots.
func (s *HoldingStage) HoldsRoots() bool { return true }

// OpenStage returns the stage itself.
func (s *HoldingStage) OpenStage(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// HoldCompleteRoots holds and reports every root.
func (s *HoldingStage) HoldCompleteRoots(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.held = append(s.held, refs...)
	held := make([]bool, len(refs))
	for i := range held {
		held[i] = true
	}
	return held, nil
}

// GetStoredBlockBatch passes the batch read to the wrapped store.
func (s *HoldingStage) GetStoredBlockBatch(ctx context.Context, refs []*block.BlockRef, fn block.StoredBlockFunc) error {
	return block.GetStoredBlockBatch(ctx, s.StoreOps, refs, fn)
}

// RequireHold fails t unless the result and the recorded calls hold exactly ref.
func (s *HoldingStage) RequireHold(t testing.TB, held []bool, ref *block.BlockRef) {
	t.Helper()
	if len(held) != 1 || !held[0] || len(s.held) != 1 || !s.held[0].EqualVT(ref) {
		t.Fatalf("stage held %v, inner stage held %v, want the root held", held, s.held)
	}
}

// RequireStageHold fails t unless holding a root through a stage of the store
// wrap builds over a HoldingStage reaches that stage and reports the root held.
func RequireStageHold(t testing.TB, wrap func(block.StoreOps) block.StoreOps) {
	// Open a stage on the wrapped store, reporting failures at the caller.
	t.Helper()
	ctx := context.Background()
	inner := &HoldingStage{}
	stage, release, err := block.OpenStage(ctx, wrap(inner))
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Hold a root through the stage.
	ref, err := block.BuildBlockRef([]byte("held root"), &block.PutOpts{HashType: hash.HashType_HashType_SHA256})
	if err != nil {
		t.Fatal(err)
	}
	held, err := block.HoldCompleteRoots(ctx, stage, []*block.BlockRef{ref})
	if err != nil {
		t.Fatal(err)
	}
	inner.RequireHold(t, held, ref)
}

// _ is a type assertion
var _ block.RootProofs = (*HoldingStage)(nil)
