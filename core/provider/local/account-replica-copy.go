package provider_local

import (
	"context"
	"time"

	"github.com/aperturerobotics/util/routine"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/kvtx"
)

// runAccountReplicaCopy hydrates every accepted World head through the Session's
// existing DEX read-through store. Cached blocks survive cancellation and restart;
// a persisted completion record is valid only for its exact immutable head.
func (a *ProviderAccount) runAccountReplicaCopy(ctx context.Context, so sobject.SharedObject, state *p2pSyncState) error {
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

	// Replacing the head cancels and joins the previous copy before starting
	// the next one. A missing old block cannot pin replication to an old head.
	copier := routine.NewRoutineContainerWithLogger(a.le.WithField("routine", "copy-world-head"), routine.WithRetry(providerBackoff))
	copier.SetContext(ctx, false)
	defer func() {
		if exited, _ := copier.SetRoutine(nil); exited != nil {
			<-exited
		}
	}()
	var snapshot sobject.SharedObjectStateSnapshot
	var previousHead *bucket.ObjectRef
	for {
		snapshot, err = states.WaitValueChange(ctx, snapshot, nil)
		if err != nil {
			return err
		}
		if snapshot == nil {
			continue
		}
		inner, err := snapshot.GetRootInner(ctx)
		if err != nil {
			return err
		}
		if inner == nil {
			continue
		}
		head := &sobject_world_engine.InnerState{}
		if err := head.UnmarshalVT(inner.GetStateData()); err != nil {
			return err
		}
		if head.GetHeadRef() == nil || head.GetHeadRef().EqualVT(previousHead) {
			continue
		}
		previousHead = head.GetHeadRef().CloneVT()
		copier.SetRoutine(func(ctx context.Context) error {
			return a.copyAndPersistAccountWorld(ctx, so, state, local, head.GetHeadRef())
		})
	}
}

// copyAndPersistAccountWorld records completion only after the block fence.
func (a *ProviderAccount) copyAndPersistAccountWorld(ctx context.Context, so sobject.SharedObject, state *p2pSyncState, local kvtx.Store, head *bucket.ObjectRef) error {
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
	if progress.GetComplete() && progress.GetHead().EqualVT(head) {
		if err := block.SetRetainedRoot(ctx, so.GetBlockStore(), "account-world", head.GetRootRef()); err != nil {
			return err
		}
		state.publishCopyProgress(progress)
		return nil
	}
	progress = &AccountReplicaCopyState{ObjectId: so.GetSharedObjectID(), Head: head.CloneVT()}
	state.publishCopyProgress(progress)
	err = a.copyAccountWorld(ctx, so, progress, func() { state.publishCopyProgress(progress) })
	if err == nil {
		err = block.SetRetainedRoot(ctx, so.GetBlockStore(), "account-world", head.GetRootRef())
	}
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
	state.publishCopyProgress(progress)
	if err != nil {
		return err
	}
	return persistErr
}

// copyAccountWorld shares durable graph retention with World publication.
// Its private proof store belongs to the serialized account replica copy routine.
func (a *ProviderAccount) copyAccountWorld(ctx context.Context, so sobject.SharedObject, progress *AccountReplicaCopyState, changed func()) error {
	local, release, err := so.AccessLocalStateStore(ctx, "account-replica-retention", nil)
	if err != nil {
		return err
	}
	defer release()
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
