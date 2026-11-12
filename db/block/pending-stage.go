package block

import (
	"context"
	"sync/atomic"
)

// pendingStage is a stage opened through a PendingStore: puts are kept in the
// PendingStore while its bound allows and go to the inner store's stage past
// it, so a writer whose tree outgrows memory holds the overflow under the
// stage until its root does.
type pendingStage struct {
	// PendingStore serves reads and keeps puts.
	*PendingStore
	// stage takes the puts past the bound.
	stage StoreOps
	// hold holds the kept blocks this stage put until the stage is released,
	// or is nil for an overflow stage, which holds none.
	hold *pendingHold
	// stored records that a put went to stage, including one that failed
	// there.
	stored atomic.Bool
	// counted records that the stage is counted in storedStages, and released
	// that its release ran. Both are guarded by the PendingStore's mtx.
	counted, released bool
}

// markStored records that a put went to the stage. A stage that holds its
// kept blocks counts in StagesStored until its release; an overflow stage's
// writer reads Stored instead.
func (p *pendingStage) markStored() {
	// Record the put, and count a stage that holds its kept blocks once.
	p.stored.Store(true)
	if p.hold == nil {
		return
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if !p.counted && !p.released {
		p.counted = true
		p.storedStages++
	}
}

// end stops counting the stage in StagesStored.
func (p *pendingStage) end() {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if !p.released {
		p.released = true
		if p.counted {
			p.storedStages--
		}
	}
}

// PutBlock keeps a block whose refs are known while the bound allows, and
// puts any other to the stage.
func (p *pendingStage) PutBlock(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	// Hand a block without its child list to the stage.
	if !opts.GetRefsKnown() || opts.GetSync() {
		p.markStored()
		return p.stage.PutBlock(ctx, data, opts)
	}

	// Keep it while the bound allows.
	ref, err := BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	existed, err := p.PutBlockBatch(ctx, []*PutBatchEntry{{Ref: ref, Data: data, Refs: opts.GetRefs(), RefsKnown: true}})
	if err != nil {
		return nil, false, err
	}
	return ref, existed[0], nil
}

// PutBlockBatch keeps the entries the bound allows and puts the rest to the
// stage.
func (p *pendingStage) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) ([]bool, error) {
	existed, stored, err := p.put(ctx, p.stage, p.hold, entries)
	if stored {
		p.markStored()
	}
	return existed, err
}

// BeginReadOperation returns the stage view itself.
func (p *pendingStage) BeginReadOperation(context.Context) (StoreOps, func(), error) {
	return p, func() {}, nil
}

// HoldCompleteRoots makes the inner stage hold each proved root.
func (p *pendingStage) HoldCompleteRoots(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	return HoldCompleteRoots(ctx, p.stage, refs)
}

// OverflowStage is a stage PendingStore.OpenOverflowStage opened: it takes
// the puts past the bound and holds none of the kept blocks.
type OverflowStage struct {
	// pendingStage keeps puts and stores the overflow.
	*pendingStage
	// release ends the inner stage.
	release func()
}

// Stored reports whether a put went past the bound to the stage, so ending
// the stage may free a block that only it held.
func (s *OverflowStage) Stored() bool {
	return s.stored.Load()
}

// Release ends the stage. A block only it held is then freed once the grace
// period passes.
func (s *OverflowStage) Release() {
	s.release()
}

// _ is a type assertion
var (
	_ StoreOps   = (*pendingStage)(nil)
	_ RootProofs = (*pendingStage)(nil)
	_ StoreOps   = (*OverflowStage)(nil)
)
