package block

import (
	"context"
	"sync"
	"sync/atomic"
)

// pendingStage is a stage opened through a PendingStore. A keeping stage
// keeps puts in the PendingStore while its bound allows and sends them to the
// inner store's stage past it, so a writer whose tree outgrows memory holds
// the overflow under the stage until its root does. Any other sends every put
// to the inner store's stage, with the kept blocks below it.
type pendingStage struct {
	// PendingStore serves reads and keeps puts.
	*PendingStore
	// stage takes the puts past the bound.
	stage StoreOps
	// hold holds the kept blocks this stage put until the stage is released,
	// or is nil for an overflow stage, which holds none.
	hold *pendingHold
	// keep keeps puts while the bound allows instead of storing them.
	keep bool
	// stored records that a put went to stage, including one that failed
	// there.
	stored atomic.Bool
}

// PutBlock keeps a block whose refs are known while a keeping stage's bound
// allows, and puts any other to the stage.
func (p *pendingStage) PutBlock(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	// Hand a block without its child list to the stage.
	if !opts.GetRefsKnown() || opts.GetSync() {
		p.stored.Store(true)
		return p.stage.PutBlock(ctx, data, opts)
	}

	// Keep it while the bound allows, or store it with its kept children.
	ref, err := BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	existed, err := p.putVerified(ctx, []*PutBatchEntry{{Ref: ref, Data: data, Refs: opts.GetRefs(), RefsKnown: true}})
	if err != nil {
		return nil, false, err
	}
	return ref, existed[0], nil
}

// PutBlockBatch keeps the entries a keeping stage's bound allows and puts the
// rest to the stage.
func (p *pendingStage) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) ([]bool, error) {
	if err := verifyKeepable(entries); err != nil {
		return nil, err
	}
	return p.putVerified(ctx, entries)
}

// putVerified keeps or stages entries whose keepable bytes match their refs.
func (p *pendingStage) putVerified(ctx context.Context, entries []*PutBatchEntry) ([]bool, error) {
	existed, stored, err := p.put(ctx, p.stage, p.hold, p.keep, entries)
	if stored {
		p.stored.Store(true)
	}
	return existed, err
}

// ReleaseRoots drops the inner stage's holds on roots written through it. A
// kept root is not in the inner stage, which ignores it; the stage's hold
// keeps it until release.
func (p *pendingStage) ReleaseRoots(ctx context.Context, refs []*BlockRef) error {
	return ReleaseRoots(ctx, p.stage, refs)
}

// BeginReadOperation returns the stage view itself.
func (p *pendingStage) BeginReadOperation(context.Context) (StoreOps, func(), error) {
	return p, func() {}, nil
}

// HoldCompleteRoots makes the inner stage hold each proved root.
func (p *pendingStage) HoldCompleteRoots(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	return HoldCompleteRoots(ctx, p.stage, refs)
}

// lazyStage is a stage of an inner store opened on its first write, so a
// writer whose puts all fit the bound opens none.
//
// Invariant: no block reaches the inner store through a lazyStage unless the
// stage is open, since each write opens it first, under mtx, and writes
// through the stage it opened. The stage then holds the block until end, as
// an eagerly opened one would. A write after end fails with
// ErrStageReleased instead of storing a block nothing holds.
type lazyStage struct {
	// StoreOps is the inner store, which serves the reads.
	StoreOps
	// mtx guards the fields below.
	mtx sync.Mutex
	// stage is the opened stage, or nil before the first write.
	stage StoreOps
	// release ends stage, or is nil before the first write.
	release func()
	// ended records that end ran.
	ended bool
}

// open returns the stage, opening it on the first call.
func (l *lazyStage) open(ctx context.Context) (StoreOps, error) {
	// Refuse a write after end, and open the stage once.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if l.ended {
		return nil, ErrStageReleased
	}
	if l.stage == nil {
		stage, release, err := OpenStage(ctx, l.StoreOps)
		if err != nil {
			return nil, err
		}
		l.stage, l.release = stage, release
	}
	return l.stage, nil
}

// PutBlock opens the stage and puts the block through it.
func (l *lazyStage) PutBlock(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	stage, err := l.open(ctx)
	if err != nil {
		return nil, false, err
	}
	return stage.PutBlock(ctx, data, opts)
}

// PutBlockBatch opens the stage and puts the entries through it.
func (l *lazyStage) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) ([]bool, error) {
	stage, err := l.open(ctx)
	if err != nil {
		return nil, err
	}
	return stage.PutBlockBatch(ctx, entries)
}

// HoldCompleteRoots opens the stage and makes it hold each proved root.
func (l *lazyStage) HoldCompleteRoots(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	stage, err := l.open(ctx)
	if err != nil {
		return nil, err
	}
	return HoldCompleteRoots(ctx, stage, refs)
}

// end ends the stage if it opened, and fails every later write.
func (l *lazyStage) end() {
	// Fail later writes, then end a stage that opened.
	l.mtx.Lock()
	release := l.release
	l.ended, l.stage, l.release = true, nil, nil
	l.mtx.Unlock()
	if release != nil {
		release()
	}
}

// OverflowStage is a stage PendingStore.OpenOverflowStage opened: it takes
// the puts past the bound and holds none of the kept blocks. The inner stage
// opens on the first put past the bound, so a writer that stays within it
// makes no call on the inner store.
type OverflowStage struct {
	// pendingStage keeps puts and stores the overflow.
	*pendingStage
	// lazy is the inner stage.
	lazy *lazyStage
}

// Stored reports whether a put went past the bound to the stage, so ending
// the stage may free a block that only it held.
func (s *OverflowStage) Stored() bool {
	return s.stored.Load()
}

// HoldCompleteRoots opens the inner stage and makes it hold each proved root.
func (s *OverflowStage) HoldCompleteRoots(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	return s.lazy.HoldCompleteRoots(ctx, refs)
}

// Release ends the stage. A block only it held is then freed once the grace
// period passes, and a later put past the bound fails with ErrStageReleased.
// Release of a stage no put went past ends nothing on the inner store.
func (s *OverflowStage) Release() {
	s.lazy.end()
}

// _ is a type assertion
var (
	_ StoreOps   = (*pendingStage)(nil)
	_ RootProofs = (*pendingStage)(nil)
	_ StoreOps   = (*OverflowStage)(nil)
	_ RootProofs = (*OverflowStage)(nil)
)
