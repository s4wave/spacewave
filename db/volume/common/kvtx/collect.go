package kvtx

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
)

// Collect runs one collection pass of the Volume, as block_gc.Collector
// describes: it renews the process lease and carries its deferred
// releases, releases lapsed leases, nets the logs and, when the store
// Config turns frees on and the Volume is adopted, frees what lost its last
// holder. A Volume without atomic commits keeps no liveness and collects
// nothing.
func (v *Volume) Collect(ctx context.Context) (block_gc.CollectResult, error) {
	// A Volume without atomic commits collects nothing.
	if v.collector == nil {
		return block_gc.CollectResult{}, nil
	}

	// Renew the process lease first, so after a stall longer than the lease
	// the pass releases no root this process still holds, then carry the deferred
	// releases of pins and stages, so the pass sees what this process
	// released, then collect.
	ctx = block.WithBackground(ctx)
	if _, err := v.renewRoots(ctx, false); err != nil {
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

// ViewBlocks reads records and raw block existence in one physical snapshot.
func (s collectorStore) ViewBlocks(ctx context.Context, fn func(rec *block_gc.Records, blocks block.StoreOps) error) error {
	// Open one reader shared by the liveness and block views.
	tx, err := s.v.kvtxStore.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if btx, ok := tx.(BlockTx); ok {
		return fn(OpenRecords(tx), btx.GetBlockStore())
	}
	return fn(OpenRecords(tx), block_store_kvtx.NewKVTxBlock(s.v.kvKey, kvtx.NewTxStore(tx), s.v.GetHashType()))
}

// BlockStored reports whether the Volume holds the bytes of ref, unmarked or
// not: the collector asks of bytes, not of a list.
func (s collectorStore) BlockStored(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return s.v.Store.GetBlockExists(ctx, ref)
}

// ScanBlocks calls fn with runs of the blocks the Volume keeps as records, as
// scanRuns reads them. A key that encodes no ref is skipped. A Volume whose
// blocks a BlockDomain serves lists none.
func (s collectorStore) ScanBlocks(ctx context.Context, fn func(refs []*block.BlockRef) error) error {
	// A BlockDomain keeps its blocks outside the records.
	if _, ok := s.v.kvtxStore.(BlockDomain); ok {
		return nil
	}

	// Read and hand over each run.
	prefix := s.v.kvKey.GetBlockFullPrefix()
	return scanRuns(ctx, s.v.kvtxStore, prefix, func(key []byte) (*block.BlockRef, bool) {
		ref := &block.BlockRef{}
		return ref, ref.UnmarshalVT(key[len(prefix):]) == nil && ref.GetHash().Validate() == nil
	}, fn)
}

// Adopt counts the holders of every block of an unadopted Volume, such as one
// that predates its logs, and marks it adopted, as block_gc.Adopt describes,
// reporting the mark phase while it runs. It runs one adoption of this Volume
// at a time, and a caller that finds one running waits for it until ctx ends.
// An adoption from another process on the same store supersedes that adoption,
// as block_gc.Adopt describes. Adopt a Volume only once every writer that uses
// it holds what it needs under named roots.
func (v *Volume) Adopt(ctx context.Context) (block_gc.AdoptResult, error) {
	// Require liveness, then adopt one run at a time.
	if !v.HoldsRoots() {
		return block_gc.AdoptResult{}, block.ErrAtomicCommitUnsupported
	}
	unlock, err := v.adoptMu.Lock(ctx)
	if err != nil {
		return block_gc.AdoptResult{}, err
	}
	defer unlock()
	defer v.enterGCPhase(volume.VolumeGCPhase_VOLUME_GC_PHASE_MARK)()
	return block_gc.Adopt(block.WithBackground(ctx), collectorStore{v: v}, v.now)
}

// runBackgroundAdopt adopts the Volume, in Adopt's bounded background slices,
// each time wakeAdopt asks, until ctx ends. A failed pass, such as one that
// finds a named block stored without a child list, waits for the next request.
// An adoption another process runs supersedes a pass and finishes the work.
func (v *Volume) runBackgroundAdopt(ctx context.Context) {
	defer close(v.adoptDone)
	for {
		// Wait for a request.
		select {
		case <-ctx.Done():
			return
		case <-v.adoptWake:
		}

		// Adopt and log the outcome.
		res, err := v.Adopt(ctx)
		switch {
		case v.le == nil || ctx.Err() != nil:
		case errors.Is(err, block_gc.ErrAdoptSuperseded):
			v.le.WithError(err).Info("background adoption of the volume left to another run")
		case err != nil:
			v.le.WithError(err).Warn("background adoption of the volume failed")
		case !res.Already:
			v.le.WithField("blocks", res.Blocks).WithField("queued", res.Queued).Info("adopted the volume")
		}
	}
}

// wakeAdopt asks the background adoption for a pass, keeping at most one
// request pending. A Volume without background adoption ignores the request.
func (v *Volume) wakeAdopt() {
	select {
	case v.adoptWake <- struct{}{}:
	default:
	}
}

// Audit walks from the Volume's named roots in one snapshot and compares the
// walk with the counts and the free queue, as block_gc.Audit describes,
// reporting the mark phase while it runs. It frees nothing, and a read-only
// Volume's audit records no holes.
func (v *Volume) Audit(ctx context.Context) (block_gc.AuditResult, error) {
	if !v.HoldsRoots() {
		return block_gc.AuditResult{}, block.ErrAtomicCommitUnsupported
	}
	defer v.enterGCPhase(volume.VolumeGCPhase_VOLUME_GC_PHASE_MARK)()
	return block_gc.Audit(block.WithBackground(ctx), collectorStore{v: v}, v.now(), !v.readOnly)
}

// WriteOffHoles writes off the recorded hole of each block in refs, so it no
// longer holds the settle's flip, as block_gc.WriteOffHoles describes with
// what the write-off gives up. It writes nothing when a ref has no hole
// record. An operator's stopped-volume command calls it.
func (v *Volume) WriteOffHoles(ctx context.Context, refs []*block.BlockRef) (block_gc.WriteOffResult, error) {
	if !v.HoldsRoots() {
		return block_gc.WriteOffResult{}, block.ErrAtomicCommitUnsupported
	}
	return block_gc.WriteOffHoles(block.WithBackground(ctx), collectorStore{v: v}, refs)
}

// SettleStatus reads what the settle's flip checks find, counting the front
// and the recorded holes up to limit each, as block_gc.SettleStatus
// describes. It writes nothing.
func (v *Volume) SettleStatus(ctx context.Context, limit int) (block_gc.SettleStatus, error) {
	if !v.HoldsRoots() {
		return block_gc.SettleStatus{}, block.ErrAtomicCommitUnsupported
	}
	return v.collector.SettleStatus(block.WithBackground(ctx), limit)
}

// AuditWalk runs the streaming audit of the Volume, as block_gc.Walk
// describes, keeping its records in scratch, a store of the walk's own and
// never the Volume's. It reads the Volume only in short views, so it runs
// beside writers and collection passes, and writes nothing to it. A nil
// opts.Now uses the Volume's clock.
func (v *Volume) AuditWalk(ctx context.Context, scratch kvtx.Store, opts block_gc.WalkOptions) (*block_gc.WalkResult, error) {
	if !v.HoldsRoots() {
		return nil, block.ErrAtomicCommitUnsupported
	}
	if opts.Now == nil {
		opts.Now = v.now
	}
	defer v.enterGCPhase(volume.VolumeGCPhase_VOLUME_GC_PHASE_MARK)()
	return block_gc.Walk(block.WithBackground(ctx), collectorStore{v: v}, scratch, opts)
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

// InspectHolders streams the structure of refs and their parent lists from a read-only snapshot.
// It retains at most eight parents per ref for each following level.
func (v *Volume) InspectHolders(
	ctx context.Context, refs []*block.BlockRef, up int,
	visit func(int, *block.BlockRef, block_gc.HolderRow) error,
) ([]block_gc.HolderLevel, error) {
	// Require liveness before opening the diagnostic snapshot.
	if !v.HoldsRoots() {
		return nil, block.ErrAtomicCommitUnsupported
	}
	return v.collector.InspectHolders(ctx, refs, up, visit)
}

// _ is a type assertion
var (
	_ block_gc.AdoptStore = collectorStore{}
	_ volume.Collector    = (*Volume)(nil)
)
