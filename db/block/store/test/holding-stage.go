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

	// Held lists the roots held, in call order.
	Held []*block.BlockRef
}

// HoldsRoots reports that the stage holds roots.
func (s *HoldingStage) HoldsRoots() bool { return true }

// OpenStage returns the stage itself.
func (s *HoldingStage) OpenStage(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// HoldCompleteRoots holds and reports every root.
func (s *HoldingStage) HoldCompleteRoots(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.Held = append(s.Held, refs...)
	held := make([]bool, len(refs))
	for i := range held {
		held[i] = true
	}
	return held, nil
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
	if len(held) != 1 || !held[0] || len(inner.Held) != 1 || !inner.Held[0].EqualVT(ref) {
		t.Fatalf("stage held %v, inner stage held %v, want the root held", held, inner.Held)
	}
}

// _ is a type assertion
var _ block.RootProofs = (*HoldingStage)(nil)
