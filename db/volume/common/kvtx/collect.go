package kvtx

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
)

// Collect runs one collection pass of the Volume, as block_gc.Collector
// describes: it renews this process's leases and carries its deferred
// releases, releases lapsed leases, nets the journal and, when the store
// Config turns frees on and the Volume is adopted, frees what lost its last
// holder. A Volume without atomic commits keeps no liveness and collects
// nothing.
func (v *Volume) Collect(ctx context.Context) (block_gc.CollectResult, error) {
	// A Volume without atomic commits collects nothing.
	if v.collector == nil {
		return block_gc.CollectResult{}, nil
	}

	// Renew this process's leases first, so after a stall longer than the
	// lease the pass releases none it still holds, then carry the deferred
	// releases of pins and stages, so the pass sees what this process
	// released, then collect.
	ctx = backgroundPass(ctx)
	if err := v.renewRoots(ctx); err != nil {
		return block_gc.CollectResult{}, err
	}
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

// BlockStored reports whether the Volume holds the bytes of ref.
func (s collectorStore) BlockStored(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return s.v.Store.GetBlockExists(ctx, ref)
}

// ScanBlocks calls fn with runs of the blocks the Volume keeps as records,
// each run read in a short snapshot after the last key the run before
// reached. A Volume whose blocks a BlockDomain serves lists none.
func (s collectorStore) ScanBlocks(ctx context.Context, fn func(refs []*block.BlockRef) error) error {
	// A BlockDomain keeps its blocks outside the records.
	if _, ok := s.v.kvtxStore.(BlockDomain); ok {
		return nil
	}

	// Read and hand over each run.
	prefix := s.v.kvKey.GetBlockFullPrefix()
	var after []byte
	for {
		refs, last, err := s.v.readBlockRun(ctx, prefix, after, legacyRootRun)
		if err != nil || last == nil {
			return err
		}
		after = last
		if err := fn(refs); err != nil {
			return err
		}
	}
}

// readBlockRun reads up to limit block keys under prefix after the key after,
// nil for the first, in one snapshot, and returns the refs they name and the
// last key, nil when none remain. A key that names no ref is skipped.
func (v *Volume) readBlockRun(ctx context.Context, prefix, after []byte, limit int) ([]*block.BlockRef, []byte, error) {
	// Position a sorted iterator past after.
	tx, err := v.kvtxStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Discard()
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if err := seekPastKey(it, after); err != nil {
		return nil, nil, err
	}

	// Parse the keys of the run.
	var refs []*block.BlockRef
	var last []byte
	for n := 0; it.Valid() && n < limit; it.Next() {
		last = bytes.Clone(it.Key())
		n++
		ref := &block.BlockRef{}
		if ref.UnmarshalVT(last[len(prefix):]) == nil && ref.GetHash().Validate() == nil {
			refs = append(refs, ref)
		}
	}
	return refs, last, it.Err()
}

// Adopt counts the holders of every block of a Volume that predates its
// journal and marks it adopted, as block_gc.Adopt describes, reporting the
// mark phase while it runs. It runs one adoption of this Volume at a time; an
// adoption from another process on the same store supersedes it, as
// block_gc.Adopt describes. Adopt a Volume only once every writer that uses
// it holds what it needs under named roots.
func (v *Volume) Adopt(ctx context.Context) (block_gc.AdoptResult, error) {
	// Require liveness, then adopt one run at a time.
	if !v.SupportsAtomicPublication() {
		return block_gc.AdoptResult{}, block.ErrAtomicPublicationUnsupported
	}
	v.adoptMu.Lock()
	defer v.adoptMu.Unlock()
	defer v.enterGCPhase(volume.VolumeGCPhase_VOLUME_GC_PHASE_MARK)()
	return block_gc.Adopt(backgroundPass(ctx), collectorStore{v: v})
}

// Audit walks from the Volume's named roots in one snapshot and compares the
// walk with the counts and the free queue, as block_gc.Audit describes,
// reporting the mark phase while it runs. It frees nothing.
func (v *Volume) Audit(ctx context.Context) (block_gc.AuditResult, error) {
	if !v.SupportsAtomicPublication() {
		return block_gc.AuditResult{}, block.ErrAtomicPublicationUnsupported
	}
	defer v.enterGCPhase(volume.VolumeGCPhase_VOLUME_GC_PHASE_MARK)()
	return block_gc.Audit(backgroundPass(ctx), collectorStore{v: v})
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

// backgroundPass marks ctx as background work: its reads are background
// reads, and its write transactions queue behind the writers that wait, as
// kvtx.WriterQueueStore admits them.
func backgroundPass(ctx context.Context) context.Context {
	return kvtx.WithBackgroundWriteContext(block.WithBackground(ctx))
}

// _ is a type assertion
var (
	_ block_gc.AdoptStore = collectorStore{}
	_ volume.Collector    = (*Volume)(nil)
)
