package world_block

import (
	"context"
	"errors"
	"sync"

	pkgerrors "github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// The producer's prepared roots and completion goroutines are bounded in
// addition to the shared block and volume admission budgets. This is not a
// batching timer: an idle volume is allowed to persist immediately.
const maxEnginePublications = 32

// engineWriteSession holds one coordinator authority across accepted revisions.
// Mutable fields are guarded by Engine.bcast. leaseMu serializes coordinator
// methods: some implementations cache a remap generation within a lease.
type engineWriteSession struct {
	lease    coord.WriteLease
	leaseMu  sync.Mutex
	prepared *bucket.ObjectRef
	tail     *enginePublication
	pending  int
	failed   error
	// fallbackPending holds admission while a legacy (non-atomic) publication
	// completes, because successors must order after its physical writes.
	fallbackPending bool
	// retiring marks authority scheduled for exactly-once release.
	retiring bool
}

type enginePublication struct {
	root     *bucket.ObjectRef
	previous *enginePublication
	durable  *block.PublicationReceipt
	complete *block.PublicationReceipt
	session  *engineWriteSession
	batch    *block.PendingBatch
}

// resumeWriteSession requires the engine's single-writer slot, which the
// caller holds until releaseWriter. It either constructs the next private
// revision or waits for failed, retiring, or blocked authority to be released.
// Completion never needs that writer slot.
func (e *Engine) resumeWriteSession(ctx context.Context, releaseWriter func()) (*EngineTx, bool, error) {
	for {
		locked := e.bcast.Lock()
		if e.closed {
			locked.Unlock()
			return nil, true, ErrEngineClosed
		}
		s := e.writeSession
		if s == nil {
			locked.Unlock()
			return nil, false, nil
		}
		if s.failed != nil || s.retiring || s.fallbackPending || s.pending >= maxEnginePublications {
			wait := locked.WaitCh()
			locked.Unlock()
			select {
			case <-ctx.Done():
				return nil, true, ctx.Err()
			case <-wait:
				continue
			}
		}
		root, err := e.baseRoot.FollowRef(ctx, s.prepared)
		if err != nil {
			locked.Unlock()
			return nil, true, err
		}
		state, err := e.buildWorldStateForRoot(ctx, false, root, e.stagedStore)
		root.Release()
		if err != nil {
			locked.Unlock()
			return nil, true, err
		}
		out := newEngineTx(e, NewTx(state))
		out.baseHeadRef = s.prepared.Clone()
		out.staged, out.session = e.stagedStore, s
		e.writeTx, e.writeTxRel = out, releaseWriter
		locked.Unlock()
		return out, true, nil
	}
}

// takeWriteSessionRetirementLocked marks authority for exactly-once release.
// It remains registered until external release finishes, preventing a new
// writer from racing a remap or reusing a lease that is being released.
func (e *Engine) takeWriteSessionRetirementLocked(s *engineWriteSession) *engineWriteSession {
	if s == nil || e.writeSession != s || s.retiring || s.pending != 0 || (e.writeTx != nil && e.writeTx.session == s) {
		return nil
	}
	s.retiring = true
	return s
}

func (e *Engine) releaseWriteSession(s *engineWriteSession) error {
	s.leaseMu.Lock()
	var err error
	if s.lease != nil {
		err = s.lease.Release(context.Background())
	}
	s.leaseMu.Unlock()
	locked := e.bcast.Lock()
	if e.writeSession == s {
		e.writeSession = nil
	}
	locked.Broadcast()
	locked.Unlock()
	return err
}

// Submit seals a revision and returns its explicit completion fence. A failed
// admission retains staged content for independently owned cursors and retries,
// but does not advance the private revision or invalidate prior accepted work.
func (e *EngineTx) Submit(ctx context.Context) (world.CommitReceipt, error) {
	_, receipt, err := e.SubmitBlockTransaction(ctx)
	return receipt, err
}

// SubmitBlockTransaction is Submit with the immutable prepared root identity.
// The returned reference is not a promise of durability: await the receipt.
func (e *EngineTx) SubmitBlockTransaction(ctx context.Context) (*bucket.ObjectRef, world.CommitReceipt, error) {
	if e.staged == nil || e.session == nil {
		root, err := e.CommitBlockTransaction(ctx)
		if err != nil {
			return nil, nil, err
		}
		r := block.NewPublicationReceipt()
		r.Resolve(nil)
		return root, r, nil
	}
	if e.writeTx == nil {
		e.Discard()
		return nil, nil, tx.ErrNotWrite
	}
	if e.rel.Swap(true) {
		return nil, nil, tx.ErrDiscarded
	}
	s := e.session
	locked := e.engine.bcast.Lock()
	if e.engine.writeTx != e || e.engine.closed {
		locked.Unlock()
		return nil, nil, tx.ErrDiscarded
	}
	e.engine.committing++ // baseRoot stays alive through preparation and completion
	locked.Unlock()
	transferred := false
	defer func() {
		if !transferred {
			e.engine.finishCommit()
		}
	}()

	root, err := e.writeTx.CommitBlockTransaction(ctx)
	if isCoordinatedWriteSnapshotError(err) {
		err = pkgerrors.Wrap(coord.ErrStaleGeneration, "prepare world blocks")
	}
	if err == nil {
		err = root.Validate(false)
	}
	if err == nil && s.lease != nil {
		s.leaseMu.Lock()
		_, err = s.lease.Refresh(ctx)
		s.leaseMu.Unlock()
	}
	// Recheck authority after the sealing operation, which can race Close or a
	// failed predecessor. No failed/stale attempt may fall back to old commit.
	locked = e.engine.bcast.Lock()
	if err == nil && (e.engine.closed || e.engine.writeTx != e || e.engine.writeSession != s) {
		err = tx.ErrDiscarded
	}
	if err == nil && s.failed != nil {
		err = block.NewPublicationDependencyError(s.failed)
	}
	var batch *block.PendingBatch
	if err == nil {
		batch, err = e.staged.TakePending(ctx)
	}
	var next *bucket.ObjectRef
	var durable *block.PublicationReceipt
	var fallback bool
	if err == nil {
		next = e.baseHeadRef.Clone()
		next.RootRef = root.Clone()
		publication := &block.AtomicPublication{
			Entries:  batch.Entries,
			Head:     e.engine.atomicHeadFn(e.baseHeadRef, next),
			RootName: e.engine.retainedRootName(),
			Root:     root.Clone(),
			Validate: func(ctx context.Context, store block.StoreOps) error {
				return e.engine.validatePreparedRoot(ctx, next, store)
			},
		}
		if s.tail != nil {
			publication.After = s.tail.durable
		}
		// Queue backpressure may wait for a physical completion, which does not
		// acquire bcast. Keep authority stable through admission. Completion
		// returns the staged borrow before acquiring bcast as well.
		durable, err = e.engine.atomicPublisher.SubmitAtomic(ctx, publication)
		if errors.Is(err, block.ErrAtomicPublicationUnsupported) {
			// This is the only error allowed to select durable-on-write behavior.
			// Block successors until that non-atomic physical sequence completes.
			fallback, err = true, nil
			durable = block.NewPublicationReceipt()
		}
	}
	if err != nil {
		if batch != nil {
			batch.Complete(err)
		}
		var retired engineRetirement
		if e.engine.writeTx == e {
			retired = e.engine.beginRetirementLocked(e.detachLocked())
		}
		locked.Unlock()
		_ = e.engine.drainRetirement(context.Background(), retired)
		return nil, nil, err
	}

	p := &enginePublication{
		root:     next,
		previous: s.tail,
		durable:  durable,
		complete: block.NewPublicationReceipt(),
		session:  s,
		batch:    batch,
	}
	s.pending++
	s.prepared, s.tail = next.Clone(), p
	s.fallbackPending = fallback
	e.engine.submitted = p.complete
	// Increment pending before detaching so the session cannot be released
	// between sealing N and beginning its durable completion.
	retired := e.engine.beginRetirementLocked(e.detachLocked())
	locked.Broadcast()
	locked.Unlock()
	transferred = true
	// Drain the old mutable state before allowing completion to release shared
	// coordinator authority. A next writer can then prepare while the durable
	// result is pending. Committing retains baseRoot across this handoff.
	_ = e.engine.drainRetirement(context.Background(), retired)
	// Completion goroutines outlive the request context: admission already
	// committed the caller to the durable result.
	durableCtx := context.WithoutCancel(ctx)
	if fallback {
		go e.engine.persistLegacyPublication(durableCtx, p, e.baseHeadRef.Clone())
	}
	go e.engine.finishPublication(durableCtx, p)
	return next.Clone(), p.complete, nil
}

// persistLegacyPublication preserves the old preparation/Sync/CAS ordering only
// for a publisher that explicitly refuses before admission. There is no retry
// through this path after a comparison, storage, or uncertain-result failure.
func (e *Engine) persistLegacyPublication(ctx context.Context, p *enginePublication, base *bucket.ObjectRef) {
	var err error
	if p.previous != nil {
		err = p.previous.complete.Wait(ctx)
	}
	if err == nil {
		err = e.writeBlockStore.PutBlockBatch(ctx, p.batch.Entries)
	}
	if err == nil {
		_, err = e.writeBlockStore.Sync(ctx)
	}
	if err == nil {
		err = block.MarkRootComplete(ctx, e.writeBlockStore, p.root.GetRootRef())
	}
	if err == nil {
		err = e.validatePreparedRoot(ctx, p.root, e.writeBlockStore)
	}
	if err == nil {
		locked := e.bcast.Lock()
		if e.commitFn != nil {
			err = e.commitFn(ctx, base, p.root.Clone())
		}
		if err == nil {
			err = block.SetRetainedRoot(ctx, e.writeBlockStore, e.retainedRootName(), p.root.GetRootRef())
		}
		locked.Unlock()
	}
	p.durable.Resolve(err)
}

// finishPublication awaits durable admission, installs the new canonical head
// in order, and resolves the caller-facing completion receipt.
func (e *Engine) finishPublication(ctx context.Context, p *enginePublication) {
	defer e.finishCommit()
	err := p.durable.Wait(ctx)
	// Return the immutable borrow even while the publication guard is held by
	// a capacity-bound producer. Raw durability is sufficient for block reads.
	p.batch.Complete(err)
	// Completed revisions retain only identity/receipts while an active
	// successor holds the session. Do not pin an already returned data borrow.
	p.batch = nil
	if p.previous != nil {
		if prior := p.previous.complete.Wait(ctx); err == nil && prior != nil {
			err = block.NewPublicationDependencyError(prior)
		}
	}
	p.previous = nil // do not retain an unbounded completed revision chain
	s := p.session
	locked := e.bcast.Lock()
	if err == nil && s.failed != nil {
		err = block.NewPublicationDependencyError(s.failed)
	}
	var retired engineRetirement
	if err == nil && !e.closed {
		retired, err = e.installRootRefLocked(ctx, p.root, false)
		if err == nil {
			e.durableHeadRef = p.root.Clone()
		}
	}
	locked.Unlock()
	_ = e.drainRetirement(ctx, retired)
	// The installed reader pin now replaces temporary publication ownership.
	// Release on errors and Engine.Close too; the durable head owns live data.
	p.durable.Release()
	if err == nil && s.lease != nil {
		s.leaseMu.Lock()
		_, err = s.lease.Publish(ctx, coord.Event{
			KeyPrefixChanged: append([]byte(nil), e.writeCoordKeyPrefix...),
			RootChanged:      p.root.Clone(),
		})
		s.leaseMu.Unlock()
	}
	locked = e.bcast.Lock()
	s.pending--
	retired = engineRetirement{}
	if err != nil {
		if s.failed == nil {
			s.failed = err
		}
		if e.writeTx != nil && e.writeTx.session == s {
			retired = e.writeTx.detachLocked()
		}
	}
	if retired.session == nil {
		retired.session = e.takeWriteSessionRetirementLocked(s)
	}
	retired = e.beginRetirementLocked(retired)
	locked.Broadcast()
	locked.Unlock()
	if releaseErr := e.drainRetirement(ctx, retired); err == nil {
		err = releaseErr
	}
	p.complete.Resolve(err)
}

var _ world.SubmittableTx = (*EngineTx)(nil)
