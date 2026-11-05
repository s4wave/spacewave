package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume"
)

// Collect runs one collection pass of the Volume, as block_gc.Collector
// describes: it carries this process's deferred releases, releases lapsed
// leases, nets the journal and, when the store Config turns frees on and the
// Volume is adopted, frees what lost its last holder. A Volume without atomic commits keeps no liveness and collects
// nothing.
func (v *Volume) Collect(ctx context.Context) (block_gc.CollectResult, error) {
	// A Volume without atomic commits collects nothing.
	if v.collector == nil {
		return block_gc.CollectResult{}, nil
	}

	// Carry the deferred releases of pins and stages first, so the pass sees
	// what this process released, then collect.
	ctx = block.WithBackground(ctx)
	if err := v.withDirectAtomic(ctx, nil); err != nil {
		return block_gc.CollectResult{}, err
	}
	return v.collector.Collect(ctx)
}

// collectorStore runs the collector's transactions in the Volume's physical
// transaction domain.
type collectorStore struct {
	// v is the Volume.
	v *Volume
}

// Update runs fn in one direct physical transaction.
func (s collectorStore) Update(ctx context.Context, fn func(rec *block_gc.Records, blocks block.StoreOps) error) error {
	return s.v.withDirectAtomic(ctx, func(p *physicalTx) error {
		return fn(p.rec, p.blocks)
	})
}

// View runs fn in one read snapshot of the liveness records.
func (s collectorStore) View(ctx context.Context, fn func(rec *block_gc.Records) error) error {
	return s.v.readRecords(ctx, fn)
}

// WritersWaiting reports whether a writer waits for the store's writer.
func (s collectorStore) WritersWaiting() bool {
	return s.v.writersWaiting()
}

// BlockStored reports whether the Volume holds the bytes of ref.
func (s collectorStore) BlockStored(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return s.v.Store.GetBlockExists(ctx, ref)
}

// Adopt counts the holders of every block of a Volume that predates its
// journal and marks it adopted, as block_gc.Adopt describes, reporting the
// mark phase while it runs. Adopt a Volume only once every writer that uses
// it holds what it needs under named roots.
func (v *Volume) Adopt(ctx context.Context) (block_gc.AdoptResult, error) {
	if !v.SupportsAtomicPublication() {
		return block_gc.AdoptResult{}, block.ErrAtomicPublicationUnsupported
	}
	defer v.enterGCPhase(volume.VolumeGCPhase_VOLUME_GC_PHASE_MARK)()
	return block_gc.Adopt(block.WithBackground(ctx), collectorStore{v: v})
}

// Audit walks from the Volume's named roots in one snapshot and compares the
// walk with the counts and the free queue, as block_gc.Audit describes,
// reporting the mark phase while it runs. It frees nothing.
func (v *Volume) Audit(ctx context.Context) (block_gc.AuditResult, error) {
	if !v.SupportsAtomicPublication() {
		return block_gc.AuditResult{}, block.ErrAtomicPublicationUnsupported
	}
	defer v.enterGCPhase(volume.VolumeGCPhase_VOLUME_GC_PHASE_MARK)()
	return block_gc.Audit(block.WithBackground(ctx), collectorStore{v: v})
}

// collectorPhase reports a collection step in the Volume's health.
func (v *Volume) collectorPhase(p block_gc.Phase) func() {
	switch p {
	case block_gc.PhaseNet:
		return v.enterGCPhase(volume.VolumeGCPhase_VOLUME_GC_PHASE_NET)
	case block_gc.PhaseSweep:
		return v.enterGCPhase(volume.VolumeGCPhase_VOLUME_GC_PHASE_SWEEP)
	}
	return func() {}
}

// _ is a type assertion
var (
	_ block_gc.AdoptStore = collectorStore{}
	_ volume.Collector    = (*Volume)(nil)
)
