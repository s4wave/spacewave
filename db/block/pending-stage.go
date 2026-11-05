package block

import (
	"context"
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
	// keys are the kept blocks this stage put, which Sweep keeps until the
	// stage is released. Guarded by the PendingStore's mtx.
	keys map[string]struct{}
}

// PutBlock keeps a block whose refs are known while the bound allows, and
// puts any other to the stage.
func (p *pendingStage) PutBlock(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	// Hand a block without its child list to the stage.
	if !opts.GetRefsKnown() || opts.GetSync() {
		return p.stage.PutBlock(ctx, data, opts)
	}

	// Keep it while the bound allows.
	ref, err := BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	existed, err := p.put(ctx, p.stage, p, []*PutBatchEntry{{Ref: ref, Data: data, Refs: opts.GetRefs(), RefsKnown: true}})
	if err != nil {
		return nil, false, err
	}
	return ref, existed[0], nil
}

// PutBlockBatch keeps the entries the bound allows and puts the rest to the
// stage.
func (p *pendingStage) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) ([]bool, error) {
	return p.put(ctx, p.stage, p, entries)
}

// BeginReadOperation returns the stage view itself.
func (p *pendingStage) BeginReadOperation(context.Context) (StoreOps, func(), error) {
	return p, func() {}, nil
}

// _ is a type assertion
var _ StoreOps = (*pendingStage)(nil)
