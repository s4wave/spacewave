package provider_local

import (
	"bytes"
	"context"
	"time"

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

// runAccountReplicaCopy hydrates the published graph of every accepted state
// through the Session's existing DEX read-through store: the checkpoint's World
// and the payloads of the operations above it, which together hold every block
// the World after those operations reads. Nothing is replayed; the Worlds the
// operations derive are rebuilt from these blocks by whoever replays them.
// Cached blocks survive cancellation and restart; a persisted completion record
// is valid only for its exact immutable head.
//
// The copier finishes each head before it takes the newest, so a writer that
// publishes faster than a copy completes does not restart it. A block that no
// connected peer holds, or holds only without its refs, is not a failure: the
// copy logs it once and waits for a newer head, which replaces it, or for a peer
// session to start on exchange that may hold the block, or an accepted config
// change that may grant access on the existing session. So a missing old block
// cannot pin replication to an old head. exchange is nil when the bucket has no
// DEX controller. Any other failure is published as the object's copy error
// before the routine retries.
func (a *ProviderAccount) runAccountReplicaCopy(
	ctx context.Context,
	so sobject.SharedObject,
	state *p2pSyncState,
	exchange *dex_solicit.Controller,
) (rerr error) {
	// Publish a failure, such as exhausted storage, where the sync status
	// reads it: from this loop and from each copy attempt, which the copier
	// retries without returning here.
	publishErr := func(ctx context.Context, err error) error {
		if err != nil && ctx.Err() == nil {
			state.publishCopyError(so.GetSharedObjectID(), err)
		}
		return err
	}
	defer func() { _ = publishErr(ctx, rerr) }()

	// Open the copy's local progress store and the object state stream.
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

	// Copy the newest published graph in latest, then each newer one.
	latest := ccontainer.NewCContainer[*replicaHead](nil)
	copier := routine.NewRoutineContainerWithLogger(a.le.WithField("routine", "copy-world-head"), routine.WithRetry(providerBackoff))
	copier.SetRoutine(func(ctx context.Context) error {
		return publishErr(ctx, a.copyAccountWorldHeads(ctx, so, state, local, latest, exchange))
	})
	copier.SetContext(ctx, false)
	defer func() {
		if exited, _ := copier.SetRoutine(nil); exited != nil {
			<-exited
		}
	}()

	// Hand each new graph or authority configuration to the copier. A grant
	// can make missing blocks readable on the existing peer session.
	var snapshot sobject.SharedObjectStateSnapshot
	var previous *block.BlockRef
	var configHead []byte
	for {
		snapshot, err = states.WaitValueChange(ctx, snapshot, nil)
		if err != nil {
			return err
		}
		if snapshot == nil {
			continue
		}
		span, root, err := sobject_world_engine.PublishedGraph(ctx, snapshot, space_world_optypes.LookupWorldOp)
		if err != nil {
			return err
		}
		if root.GetEmpty() {
			continue
		}
		config, err := snapshot.GetConfig(ctx)
		if err != nil {
			return err
		}
		if root.EqualsRef(previous) && bytes.Equal(config.GetConfigChainHash(), configHead) {
			continue
		}
		previous, configHead = root, bytes.Clone(config.GetConfigChainHash())
		latest.SetValue(&replicaHead{span: span, head: &bucket.ObjectRef{RootRef: root}})
	}
}

// replicaHead is a published graph for the account replica copy.
type replicaHead struct {
	// span is the published span block, or nil.
	span *block.PutBatchEntry
	// head references the published World root.
	head *bucket.ObjectRef
}

// copyAccountWorldHeads copies the head in latest, then each newer one, until
// ctx ends or a copy fails. It stores the span block on each attempt, so a
// sweep between attempts cannot leave the copy without its root.
func (a *ProviderAccount) copyAccountWorldHeads(
	ctx context.Context,
	so sobject.SharedObject,
	state *p2pSyncState,
	local kvtx.Store,
	latest *ccontainer.CContainer[*replicaHead],
	exchange *dex_solicit.Controller,
) error {
	var copied *replicaHead
	for {
		// Take the newest head.
		next, err := latest.WaitValueChange(ctx, copied, nil)
		if err != nil {
			return err
		}

		// Store its span block and copy it.
		if next.span != nil {
			if _, err := so.GetBlockStore().PutBlockBatch(ctx, []*block.PutBatchEntry{next.span}); err != nil {
				return err
			}
		}
		if err := a.copyAccountWorldHead(ctx, so, state, local, latest, next, exchange); err != nil {
			return err
		}
		copied = next
	}
}

// copyAccountWorldHead copies cur until it completes or latest holds a newer
// head. A block that no reachable source holds or serves, such as one every
// peer refuses, or holds only without its refs, does not appear by retrying the
// same head: the copy logs it once and retries when a peer session starts on
// exchange or the authority config changes. A session retry first reads the
// blocks the walk found missing and walks again only once one of them arrives.
// It returns nil when a newer head arrives first, which the caller copies next.
func (a *ProviderAccount) copyAccountWorldHead(
	ctx context.Context,
	so sobject.SharedObject,
	state *p2pSyncState,
	local kvtx.Store,
	latest *ccontainer.CContainer[*replicaHead],
	cur *replicaHead,
	exchange *dex_solicit.Controller,
) error {
	for logged := false; ; logged = true {
		// Count peer sessions first so one starting during the copy still
		// wakes the wait.
		var starts uint64
		if exchange != nil {
			starts = exchange.GetSessionStarts()
		}
		err := a.copyAndPersistAccountWorld(ctx, so, state, local, cur.head)
		if !block.IsNotAvailable(err) && !errors.Is(err, block.ErrRefsUnknown) {
			return err
		}

		// Wait for a source that may hold the block with its refs, or for a
		// newer head.
		if !logged {
			a.le.WithError(err).Info("waiting for a peer that holds a World block with its refs")
		}
		var holes *worldHolesError
		errors.As(err, &holes)
		waitCtx, cancel := context.WithCancel(ctx)
		go func() {
			_, _ = latest.WaitValueChange(waitCtx, cur, nil)
			cancel()
		}()
		if exchange != nil {
			err = waitWorldHoles(waitCtx, so.GetBlockStore(), exchange, starts, holes)
		} else {
			<-waitCtx.Done()
		}
		cancel()
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		if latest.GetValue() != cur {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// waitWorldHoles waits for a peer session to start on exchange. When holes
// names the blocks a finished walk missed, it waits on until one of them
// arrives; a walk stopped by a block without its refs walks again on the next
// session.
func waitWorldHoles(
	ctx context.Context,
	store block.StoreOps,
	exchange *dex_solicit.Controller,
	starts uint64,
	holes *worldHolesError,
) error {
	for {
		if err := exchange.WaitSessionStart(ctx, starts); err != nil {
			return err
		}
		if holes == nil {
			return nil
		}
		arrived, err := holes.probe(ctx, store)
		if err != nil || arrived {
			return err
		}
		starts = exchange.GetSessionStarts()
	}
}

// worldHolesError reports the blocks a completed World walk could not read.
// The walk persisted the proofs of every complete subtree, so the next walk
// reads only the paths down to these blocks.
type worldHolesError struct {
	// refs are the missing blocks.
	refs []*block.BlockRef
}

// Error describes the missing blocks.
func (e *worldHolesError) Error() string {
	return errors.Wrapf(e.Unwrap(), "%d World blocks, first %s", len(e.refs), e.refs[0].MarshalString()).Error()
}

// Unwrap reports the holes as missing blocks.
func (e *worldHolesError) Unwrap() error {
	return block.ErrNotFound
}

// probe reads each missing block through store and reports whether any
// arrived. A block no source serves yet has not arrived.
func (e *worldHolesError) probe(ctx context.Context, store block.StoreOps) (bool, error) {
	for _, ref := range e.refs {
		_, found, err := store.GetBlock(ctx, ref)
		if block.IsNotAvailable(err) {
			continue
		}
		if err != nil || found {
			return found, err
		}
	}
	return false, nil
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
		if err := block.OverwriteRoots(ctx, so.GetBlockStore(), nil, []block.NamedRoot{{Name: "account-world", Ref: head.GetRootRef()}}); err != nil {
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
		err = block.OverwriteRoots(ctx, so.GetBlockStore(), nil, []block.NamedRoot{{Name: "account-world", Ref: head.GetRootRef()}})
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
// Its private proof store belongs to the serialized account replica copy
// routine. A missing block does not stop the walk: the walk completes, keeps
// the proofs of every complete subtree, and returns a worldHolesError.
func (a *ProviderAccount) copyAccountWorld(ctx context.Context, so sobject.SharedObject, progress *AccountReplicaCopyState, changed func()) error {
	// Open the shared retention store for the copy.
	local, release, err := so.AccessLocalStateStore(ctx, "account-replica-retention", nil)
	if err != nil {
		return err
	}
	defer release()

	// Retain the World head, publishing progress while blocks copy.
	lastUpdate := time.Now()
	visited := func(_ *block.BlockRef, data []byte) {
		progress.Blocks++
		progress.Bytes += uint64(len(data))
		if time.Since(lastUpdate) >= 250*time.Millisecond {
			changed()
			lastUpdate = time.Now()
		}
	}
	holes := &worldHolesError{}
	missing := func(ref *block.BlockRef) { holes.refs = append(holes.refs, ref.CloneVT()) }
	if err := sobject_world_engine.RetainWorld(ctx, so, progress.GetHead(), local, &sobject_world_engine.RetainOptions{Visited: visited, Missing: missing}); err != nil {
		return err
	}
	if len(holes.refs) != 0 {
		return holes
	}
	return nil
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
