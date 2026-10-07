package provider_local

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/db/kvtx"
)

// runAccountReplicaCopy hydrates the World of every accepted state through the
// Session's existing DEX read-through store. The World is the checkpoint's World
// with the operation set replayed onto it, as the World engine engineID builds
// it on b. One replay follows every state, so each pass replays only the
// operations the state added. Cached blocks survive cancellation and restart; a persisted
// completion record is valid only for its exact immutable head.
//
// A block that no connected peer holds, or holds only without its refs, is not
// a failure: the copy logs it once and waits for a new state, or for a peer
// session to start on exchange that may hold the block. exchange is nil when
// the bucket has no DEX controller. Any other failure is published as the
// object's copy error before the routine retries.
func (a *ProviderAccount) runAccountReplicaCopy(
	ctx context.Context,
	b bus.Bus,
	so sobject.SharedObject,
	engineID string,
	state *p2pSyncState,
	exchange *dex_solicit.Controller,
) (rerr error) {
	// Publish a failure, such as exhausted storage, where the sync status
	// reads it.
	defer func() {
		if rerr != nil && ctx.Err() == nil {
			state.publishCopyError(so.GetSharedObjectID(), rerr)
		}
	}()

	// Open the copy's local progress store, the object state stream and the
	// replay that follows it.
	local, release, err := so.AccessLocalStateStore(ctx, "account-replica-copy", nil)
	if err != nil {
		return err
	}
	defer release()
	states, releaseStates, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer releaseStates()
	replay, err := sobject_world_engine.NewWorldReplay(a.le, b, a.GetStepFactorySet(), so, engineID, space_world_optypes.LookupWorldOp)
	if err != nil {
		return err
	}

	// Replacing the head cancels and joins the previous copy before starting
	// the next one. A missing old block cannot pin replication to an old head.
	copier := routine.NewRoutineContainerWithLogger(a.le.WithField("routine", "copy-world-head"), routine.WithRetry(providerBackoff))
	copier.SetContext(ctx, false)
	defer func() {
		if exited, _ := copier.SetRoutine(nil); exited != nil {
			<-exited
		}
	}()

	// Copy each new World head as its state arrives.
	var snapshot sobject.SharedObjectStateSnapshot
	var previousHead *bucket.ObjectRef
	var missing bool
	var starts uint64
	for {
		// While a block is missing, a peer that connects may hold it.
		var peerStart func(context.Context) error
		if missing && exchange != nil {
			peerStart = func(ctx context.Context) error {
				return exchange.WaitSessionStart(ctx, starts)
			}
		}
		snapshot, err = waitStateOrWake(ctx, states, snapshot, peerStart)
		if err != nil {
			return err
		}
		if snapshot == nil {
			continue
		}

		// Replay the operation set onto the checkpoint's World. Members write
		// edits as operations, so the checkpoint alone lags the World they
		// hold. Count peer sessions first so one starting during the replay
		// still wakes the wait.
		if exchange != nil {
			starts = exchange.GetSessionStarts()
		}
		head, err := replay.Replay(ctx, snapshot)
		if errors.Is(err, block.ErrNotFound) {
			if !missing {
				a.le.WithError(err).Info("waiting for a peer that holds a missing World block")
			}
			missing = true
			continue
		}
		if err != nil {
			return err
		}
		missing = false

		// Start the copy for a new head.
		if head.GetHeadRef() == nil || head.GetHeadRef().EqualVT(previousHead) {
			continue
		}
		previousHead = head.GetHeadRef().CloneVT()
		copier.SetRoutine(func(ctx context.Context) error {
			return a.copyAccountWorldHead(ctx, so, state, local, head.GetHeadRef(), exchange)
		})
	}
}

// waitStateOrWake waits for a state other than snapshot. When wake is set and
// returns nil first, it returns snapshot unchanged.
func waitStateOrWake(
	ctx context.Context,
	states ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
	snapshot sobject.SharedObjectStateSnapshot,
	wake func(context.Context) error,
) (sobject.SharedObjectStateSnapshot, error) {
	// Without a wake, only a new state ends the wait.
	if wake == nil {
		return states.WaitValueChange(ctx, snapshot, nil)
	}

	// Cancel the state wait when wake returns; woke tells that cancel apart
	// from the caller's.
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	woke := make(chan struct{})
	go func() {
		if wake(waitCtx) == nil {
			close(woke)
			cancel()
		}
	}()
	next, err := states.WaitValueChange(waitCtx, snapshot, nil)
	if err != nil && ctx.Err() == nil {
		select {
		case <-woke:
			return snapshot, nil
		default:
		}
	}
	return next, err
}

// copyAccountWorldHead copies head until it completes. A block that no
// reachable source holds, or holds only without its refs, does not appear by
// retrying the same head: the copy logs it once and retries when a peer
// session starts on exchange. Without exchange it waits for the next head,
// which cancels this routine.
func (a *ProviderAccount) copyAccountWorldHead(
	ctx context.Context,
	so sobject.SharedObject,
	state *p2pSyncState,
	local kvtx.Store,
	head *bucket.ObjectRef,
	exchange *dex_solicit.Controller,
) error {
	for logged := false; ; logged = true {
		// Count peer sessions first so one starting during the copy still
		// wakes the wait.
		var starts uint64
		if exchange != nil {
			starts = exchange.GetSessionStarts()
		}
		err := a.copyAndPersistAccountWorld(ctx, so, state, local, head)
		if !errors.Is(err, block.ErrNotFound) && !errors.Is(err, block.ErrRefsUnknown) {
			return err
		}

		// Wait for a source that may hold the block with its refs.
		if !logged {
			a.le.WithError(err).Info("waiting for a peer that holds a World block with its refs")
		}
		if exchange == nil {
			<-ctx.Done()
			return context.Cause(ctx)
		}
		if err := exchange.WaitSessionStart(ctx, starts); err != nil {
			return err
		}
	}
}

// copyAndPersistAccountWorld records completion only after the block fence.
func (a *ProviderAccount) copyAndPersistAccountWorld(ctx context.Context, so sobject.SharedObject, state *p2pSyncState, local kvtx.Store, head *bucket.ObjectRef) error {
	// Read the persisted progress record for this head.
	progress := &AccountReplicaCopyState{}
	err := kvtx.RunTransaction(ctx, false, func(ctx context.Context) (kvtx.Tx, error) { return local.NewTransaction(ctx, false) }, func(ctx context.Context, tx kvtx.Tx) error {
		data, found, err := tx.Get(ctx, []byte("account-replica-copy/progress"))
		if err == nil && found {
			err = progress.UnmarshalVT(data)
		}
		return err
	})
	if err != nil {
		return err
	}

	// Retain the head and republish when this head already completed.
	if progress.GetComplete() && progress.GetHead().EqualVT(head) {
		if err := block.SetRetainedRoot(ctx, so.GetBlockStore(), "account-world", head.GetRootRef()); err != nil {
			return err
		}
		state.publishCopyProgress(progress)
		return nil
	}

	// Copy the World and retain its root on success.
	progress = &AccountReplicaCopyState{ObjectId: so.GetSharedObjectID(), Head: head.CloneVT()}
	state.publishCopyProgress(progress)
	err = a.copyAccountWorld(ctx, so, progress, func() { state.publishCopyProgress(progress) })
	if err == nil {
		err = block.SetRetainedRoot(ctx, so.GetBlockStore(), "account-world", head.GetRootRef())
	}

	// Record the outcome in the progress state.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		progress.Error = err.Error()
	} else {
		progress.Complete = true
	}

	// The block fence precedes this transaction, so a crash cannot retain
	// a completion claim whose data was still buffered.
	persistErr := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) { return local.NewTransaction(ctx, true) }, func(ctx context.Context, tx kvtx.Tx) error {
		data, err := progress.MarshalVT()
		if err != nil {
			return err
		}
		return tx.Set(ctx, []byte("account-replica-copy/progress"), data)
	})
	if persistErr != nil {
		progress.Complete = false
		progress.Error = persistErr.Error()
	}

	// Publish the final progress and report the copy or persistence error.
	state.publishCopyProgress(progress)
	if err != nil {
		return err
	}
	return persistErr
}

// copyAccountWorld shares durable graph retention with World publication.
// Its private proof store belongs to the serialized account replica copy routine.
func (a *ProviderAccount) copyAccountWorld(ctx context.Context, so sobject.SharedObject, progress *AccountReplicaCopyState, changed func()) error {
	// Open the shared retention store for the copy.
	local, release, err := so.AccessLocalStateStore(ctx, "account-replica-retention", nil)
	if err != nil {
		return err
	}
	defer release()

	// Retain the World head, publishing progress while blocks copy.
	lastUpdate := time.Now()
	return sobject_world_engine.RetainWorld(ctx, so, progress.GetHead(), local, func(_ *block.BlockRef, data []byte) {
		progress.Blocks++
		progress.Bytes += uint64(len(data))
		if time.Since(lastUpdate) >= 250*time.Millisecond {
			changed()
			lastUpdate = time.Now()
		}
	})
}

// publishCopyProgress snapshots mutable worker progress under the state lock.
func (s *p2pSyncState) publishCopyProgress(progress *AccountReplicaCopyState) {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if s.copyProgress == nil {
			s.copyProgress = make(map[string]*AccountReplicaCopyState)
		}
		s.copyProgress[progress.GetObjectId()] = progress.CloneVT()
		bcast()
	})
}

// publishCopyError records err on the object's latest copy progress. A
// complete copy stays complete: its blocks are durable.
func (s *p2pSyncState) publishCopyError(objectID string, err error) {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		// Mark the latest progress failed and wake the status watchers.
		progress := s.copyProgress[objectID].CloneVT()
		if progress == nil {
			progress = &AccountReplicaCopyState{ObjectId: objectID}
		}
		progress.Error = err.Error()
		if s.copyProgress == nil {
			s.copyProgress = make(map[string]*AccountReplicaCopyState)
		}
		s.copyProgress[objectID] = progress
		bcast()
	})
}

// GetAccountCopyProgress returns isolated progress snapshots and the next change.
func (a *ProviderAccount) GetAccountCopyProgress() ([]*AccountReplicaCopyState, <-chan struct{}) {
	var out []*AccountReplicaCopyState
	var wait <-chan struct{}
	a.p2pSyncBcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
		wait = getWait()
		if state := a.p2pSync; state != nil {
			state.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
				wait = getWait()
				for _, progress := range state.copyProgress {
					out = append(out, progress.CloneVT())
				}
			})
		}
	})
	return out, wait
}
