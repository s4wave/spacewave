package world_block

import (
	"context"
	"errors"
	"fmt"
	"sync"

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
	blocking bool // a legacy fallback must finish before further admission
	retiring bool
	done     chan struct{}
}

type enginePublication struct {
	root     *bucket.ObjectRef
	previous *enginePublication
	durable  *block.PublicationReceipt
	complete *block.PublicationReceipt
	session  *engineWriteSession
	batch    *block.PendingBatch
}

// resumeWriteSession is called with the local writer mutex held. It either
// constructs the next private revision or waits for failed/retiring authority
// to be released. Completion never needs that writer mutex.
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
		if s.failed != nil || s.retiring || s.blocking || s.pending >= maxEnginePublications {
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
	close(s.done)
	locked.Broadcast()
	locked.Unlock()
	return err
}

// Submit seals a revision and returns its explicit completion fence. A failed
// admission retains staged content for independently owned cursors and retries,
// but does not advance the private revision or invalidate prior accepted work.
func (t *EngineTx) Submit(ctx context.Context) (world.CommitReceipt, error) {
	_, receipt, err := t.SubmitBlockTransaction(ctx)
	return receipt, err
}

// SubmitBlockTransaction is Submit with the immutable prepared root identity.
// The returned reference is not a promise of durability: await the receipt.
func (t *EngineTx) SubmitBlockTransaction(ctx context.Context) (*bucket.ObjectRef, world.CommitReceipt, error) {
	if t.staged == nil || t.session == nil {
		root, err := t.CommitBlockTransaction(ctx)
		if err != nil {
			return nil, nil, err
		}
		r := block.NewPublicationReceipt()
		r.Resolve(nil)
		return root, r, nil
	}
	if t.writeTx == nil {
		t.Discard()
		return nil, nil, tx.ErrNotWrite
	}
	if t.rel.Swap(true) {
		return nil, nil, tx.ErrDiscarded
	}
	e, s := t.engine, t.session
	locked := e.bcast.Lock()
	if e.writeTx != t || e.closed {
		locked.Unlock()
		return nil, nil, tx.ErrDiscarded
	}
	e.committing++ // baseRoot stays alive through preparation and completion
	locked.Unlock()
	transferred := false
	defer func() {
		if !transferred {
			e.finishCommit()
		}
	}()

	root, err := t.writeTx.CommitBlockTransaction(ctx)
	if isCoordinatedWriteSnapshotError(err) {
		err = fmt.Errorf("%w: prepare world blocks", coord.ErrStaleGeneration)
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
	locked = e.bcast.Lock()
	if err == nil && (e.closed || e.writeTx != t || e.writeSession != s) {
		err = tx.ErrDiscarded
	}
	if err == nil && s.failed != nil {
		err = fmt.Errorf("%w: %v", block.ErrPublicationDependency, s.failed)
	}
	var batch *block.PendingBatch
	if err == nil {
		batch, err = t.staged.TakePending(ctx)
	}
	var next *bucket.ObjectRef
	var durable *block.PublicationReceipt
	var fallback bool
	if err == nil {
		next = t.baseHeadRef.Clone()
		next.RootRef = root.Clone()
		publication := &block.AtomicPublication{
			Entries: batch.Entries,
			Head:    e.atomicHeadFn(t.baseHeadRef, next),
			Validate: func(ctx context.Context, store block.StoreOps) error {
				return e.validatePreparedRoot(ctx, next, store)
			},
		}
		if s.tail != nil {
			publication.After = s.tail.durable
		}
		// Queue backpressure may wait for a physical completion, which does not
		// acquire bcast. Keep authority stable through admission. Completion
		// returns the staged borrow before acquiring bcast as well.
		durable, err = e.atomicPublisher.SubmitAtomic(ctx, publication)
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
		if e.writeTx == t {
			retired = e.beginRetirementLocked(t.detachLocked())
		}
		locked.Unlock()
		_ = e.drainRetirement(context.Background(), retired)
		return nil, nil, err
	}

	p := &enginePublication{root: next, previous: s.tail, durable: durable,
		complete: block.NewPublicationReceipt(), session: s, batch: batch}
	s.pending++
	s.prepared, s.tail = next.Clone(), p
	s.blocking = fallback
	e.submitted = p.complete
	// Increment pending before detaching so the session cannot be released
	// between sealing N and beginning its durable completion.
	retired := e.beginRetirementLocked(t.detachLocked())
	locked.Broadcast()
	locked.Unlock()
	transferred = true
	// Drain the old mutable state before allowing completion to release shared
	// coordinator authority. A next writer can then prepare while the durable
	// result is pending. Committing retains baseRoot across this handoff.
	_ = e.drainRetirement(context.Background(), retired)
	if fallback {
		go e.persistLegacyPublication(p, t.baseHeadRef.Clone())
	}
	go e.finishPublication(p)
	return next.Clone(), p.complete, nil
}

// persistLegacyPublication preserves the old preparation/Sync/CAS ordering only
// for a publisher that explicitly refuses before admission. There is no retry
// through this path after a comparison, storage, or uncertain-result failure.
func (e *Engine) persistLegacyPublication(p *enginePublication, base *bucket.ObjectRef) {
	ctx := context.Background()
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
		err = e.validatePreparedRoot(ctx, p.root, e.writeBlockStore)
	}
	if err == nil {
		locked := e.bcast.Lock()
		if e.commitFn != nil {
			err = e.commitFn(ctx, base, p.root.Clone())
		}
		locked.Unlock()
	}
	p.durable.Resolve(err)
}

func (e *Engine) finishPublication(p *enginePublication) {
	defer e.finishCommit()
	ctx := context.Background()
	err := p.durable.Wait(ctx)
	// Return the immutable borrow even while the publication guard is held by
	// a capacity-bound producer. Raw durability is sufficient for block reads.
	p.batch.Complete(err)
	if p.previous != nil {
		if prior := p.previous.complete.Wait(ctx); err == nil && prior != nil {
			err = fmt.Errorf("%w: %v", block.ErrPublicationDependency, prior)
		}
	}
	p.previous = nil // do not retain an unbounded completed revision chain
	s := p.session
	locked := e.bcast.Lock()
	if err == nil && s.failed != nil {
		err = fmt.Errorf("%w: %v", block.ErrPublicationDependency, s.failed)
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
