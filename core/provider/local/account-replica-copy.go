package provider_local

import (
	"bytes"
	"context"
	"os"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
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
// copy logs it once and waits for a peer session to start on exchange that may
// hold the block, or an accepted config change that may grant access on the
// existing session, and then copies the newest head (copyAccountWorldHead).
// exchange is nil when the bucket has no DEX controller. Any other failure is
// published as the object's copy error before the routine retries.
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

	// Reset the known-absent blocks when a peer session starts, since the new
	// peer may serve them.
	absent := &knownAbsent{}
	if exchange != nil {
		watchCtx, cancelWatch := context.WithCancel(ctx)
		watched := make(chan struct{})
		starts := exchange.GetSessionStarts()
		go func() {
			resetOnSessionStart(watchCtx, exchange, starts, absent)
			close(watched)
		}()
		defer func() {
			cancelWatch()
			<-watched
		}()
	}

	// Copy the newest published graph in latest, then each newer one.
	latest := ccontainer.NewCContainer[*replicaHead](nil)

	// Copy the newest head again after each change of the keep-full-copy
	// setting: a fresh value for the same head wakes the copier.
	setCtx, cancelSet := context.WithCancel(ctx)
	setWatched := make(chan struct{})
	go func() {
		defer close(setWatched)
		gen := a.linkedCopiesGen.GetValue()
		for {
			next, err := a.linkedCopiesGen.WaitValueChange(setCtx, gen, nil)
			if err != nil {
				return
			}
			gen = next
			latest.SwapValue(func(cur *replicaHead) *replicaHead {
				if cur == nil {
					return nil
				}
				return &replicaHead{span: cur.span, head: cur.head}
			})
		}
	}()
	defer func() {
		cancelSet()
		<-setWatched
	}()
	copier := routine.NewRoutineContainerWithLogger(a.le.WithField("routine", "copy-world-head"), routine.WithRetry(providerBackoff))
	copier.SetRoutine(func(ctx context.Context) error {
		// The copy's known-absent set resets on grants and session starts,
		// so its reads skip the store's memory of refused blocks.
		err := publishErr(ctx, a.copyAccountWorldHeads(withAbsenceTracked(ctx), so, state, local, latest, absent, exchange))
		if errors.Is(err, volume.ErrReserve) {
			// Retry once the volume leaves its disk reserve, not on a timer.
			if werr := volume.WaitWritable(ctx, a.vol); werr != nil {
				return werr
			}
		}
		return err
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
		span, root, err := space_world.PublishedGraph(ctx, snapshot, space_world_optypes.LookupWorldOp)
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
		if !bytes.Equal(config.GetConfigChainHash(), configHead) {
			absent.reset()
		}
		previous, configHead = root, bytes.Clone(config.GetConfigChainHash())
		latest.SetValue(&replicaHead{span: span, head: &bucket.ObjectRef{RootRef: root}})
	}
}

// ReplicaRewalkEnv names the environment variable that restores, when set to
// "on", the earlier wait of a copy blocked on missing blocks: each newer head
// ends the wait and is walked at once, and without a peer exchange only a newer
// head ends it. Every other value, including an unset variable, keeps the
// default, under which a blocked copy waits for a peer session start or a
// config change and then copies the newest head.
//
// What the per-head walk did besides growing the Volume, and who read it:
//
//   - It rewrote the progress record (account-replica-copy/progress) and the
//     published progress for each head. The record is read back only to skip a
//     completed head (copyAndPersistAccountWorld); the published progress
//     feeds GetAccountCopyProgress and the Session's sync status, which shows
//     its blocks, bytes, completion and error. While blocked those now stay at
//     the blocked head's walk, with the same incomplete state and hole error,
//     where they showed the newest head's walk.
//   - It logged "waiting for a peer that holds a World block with its refs"
//     once per head; now it logs once per walk that blocks, so once per wake.
//   - It stored each newer head's span block on arrival
//     (copyAccountWorldHeads); now the span is stored when the copy takes its
//     head after a wait.
const ReplicaRewalkEnv = "SPACEWAVE_REPLICA_COPY_REWALK"

// replicaRewalk selects ReplicaRewalkEnv's earlier behavior. It is read once,
// here, at startup.
var replicaRewalk = os.Getenv(ReplicaRewalkEnv) == "on"

// replicaHead is a published graph for the account replica copy.
type replicaHead struct {
	// span is the published span block, or nil.
	span *block.PutBatchEntry
	// head references the published World root.
	head *bucket.ObjectRef
}

// copyAccountWorldHeads copies the head in latest, then each newer one, until
// ctx ends or a copy fails. A stage holds the span block across the head's
// attempts, so a sweep between attempts cannot leave the copy without its
// root.
func (a *ProviderAccount) copyAccountWorldHeads(
	ctx context.Context,
	so sobject.SharedObject,
	state *p2pSyncState,
	local kvtx.Store,
	latest *ccontainer.CContainer[*replicaHead],
	absent *knownAbsent,
	exchange *dex_solicit.Controller,
) error {
	var copied *replicaHead
	for {
		// Take the newest head.
		next, err := latest.WaitValueChange(ctx, copied, nil)
		if err != nil {
			return err
		}

		// Store its span block under a stage held across the copy's attempts,
		// and copy it. The copy puts the span again under its own stage, and
		// the moved root holds it after that.
		release := func() {}
		if next.span != nil {
			staged, rel, err := block.OpenStage(ctx, so.GetBlockStore())
			if err != nil {
				return err
			}
			release = rel
			if _, err := staged.PutBlockBatch(ctx, []*block.PutBatchEntry{next.span}); err != nil {
				release()
				return err
			}
		}
		err = a.copyAccountWorldHead(ctx, so, state, local, latest, next, absent, exchange)
		release()
		if err != nil {
			return err
		}
		copied = next
	}
}

// copyAccountWorldHead copies cur until it completes, or until a wait for a
// missing block ends with a newer head in latest, which it leaves to the caller
// to copy. A block that no reachable source holds or serves, such as one every
// peer refuses, or holds only without its refs, does not appear by retrying the
// same head: the copy logs it once and retries when a peer session starts on
// exchange or the authority config changes. A retry first reads the blocks the
// walk found missing and walks again only once one of them arrives.
//
// A newer head does not end the wait (unless ReplicaRewalkEnv is on): a walk
// of it would stop at the same blocks, and every such walk puts each block on
// the paths from the root to them again, under a stage that releases them when
// the walk ends, so the Volume grew by those writes for each head. Waiting
// writes nothing. It gives up only the case of a newer head that no longer
// reaches the missing blocks, which now completes at the next session start or
// config change instead of at once. With no exchange, as when the bucket's DEX
// controller failed to start, no session start comes, and the wait ends only on
// a config change or when the copy restarts.
//
// Invariants, with either setting of ReplicaRewalkEnv:
//
//  1. Newest head wins: when a wait ends, the copy takes the newest head in
//     latest, never an older one, and a blocked head is never retried while a
//     newer one waits.
//  2. Nothing is released or freed before the account World root moves: a copy
//     with holes does not move the root (copyWorldGraph), and the wait writes
//     and releases nothing.
//  3. Completion proofs cover only subtrees with no hole under them
//     (block.StageCopy, invariant 4); the wait adds none.
func (a *ProviderAccount) copyAccountWorldHead(
	ctx context.Context,
	so sobject.SharedObject,
	state *p2pSyncState,
	local kvtx.Store,
	latest *ccontainer.CContainer[*replicaHead],
	cur *replicaHead,
	absent *knownAbsent,
	exchange *dex_solicit.Controller,
) error {
	for logged := false; ; logged = true {
		// Take the known-absent generation first so a peer session starting
		// during the copy still wakes the wait.
		gen := absent.generation()
		err := a.copyAndPersistAccountWorld(ctx, so, state, local, cur.head, absent)
		if !block.IsNotAvailable(err) && !errors.Is(err, block.ErrRefsUnknown) {
			return err
		}

		// Wait for a source that may hold the block with its refs, and under
		// ReplicaRewalkEnv also for a newer head.
		if !logged {
			a.le.WithError(err).Info("waiting for a peer that holds a World block with its refs")
		}
		var holes *worldHolesError
		errors.As(err, &holes)
		waitCtx, cancel := context.WithCancel(ctx)
		if replicaRewalk {
			go func() {
				_, _ = latest.WaitValueChange(waitCtx, cur, nil)
				cancel()
			}()
		}
		if exchange != nil || !replicaRewalk {
			err = waitWorldHoles(waitCtx, so.GetBlockStore(), absent, gen, holes)
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

// waitWorldHoles waits for absent to reset after gen, on a peer session start
// or a config change. When holes names the blocks a finished walk missed, it
// reads each once and waits on until one of them arrives, keeping the rest in
// absent so the next walk does not read them again; a walk stopped by a block
// without its refs walks again on the next reset.
func waitWorldHoles(
	ctx context.Context,
	store block.StoreOps,
	absent *knownAbsent,
	gen uint64,
	holes *worldHolesError,
) error {
	for {
		if err := absent.waitReset(ctx, gen); err != nil {
			return err
		}
		if holes == nil {
			return nil
		}
		gen = absent.generation()
		arrived, err := holes.probe(ctx, store, absent, gen)
		if err != nil || arrived {
			return err
		}
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
// arrived. A block no source serves yet has not arrived, and is added to
// absent under gen. A read that ctx ended proves nothing about the block, and
// the peer exchange reports it as unavailable, so it ends the probe without
// adding the block: a newer head cancels the wait while the reset that woke it
// is the one that can make the block servable.
func (e *worldHolesError) probe(ctx context.Context, store block.StoreOps, absent *knownAbsent, gen uint64) (bool, error) {
	for _, ref := range e.refs {
		_, found, err := store.GetBlock(ctx, ref)
		if ctx.Err() != nil {
			return false, context.Cause(ctx)
		}
		if block.IsNotAvailable(err) || (err == nil && !found) {
			absent.add(gen, ref)
			continue
		}
		if err != nil || found {
			return found, err
		}
	}
	return false, nil
}

// copyAndPersistAccountWorld records completion only after the block fence.
func (a *ProviderAccount) copyAndPersistAccountWorld(
	ctx context.Context,
	so sobject.SharedObject,
	state *p2pSyncState,
	local kvtx.Store,
	head *bucket.ObjectRef,
	absent *knownAbsent,
) error {
	// Read the persisted progress record for this head.
	progress := &AccountReplicaCopyState{}
	openRead := func(ctx context.Context) (kvtx.Tx, error) { return local.NewTransaction(ctx, false) }
	err := kvtx.RunTransaction(ctx, false, openRead, func(ctx context.Context, tx kvtx.Tx) error {
		data, found, err := tx.Get(ctx, []byte("account-replica-copy/progress"))
		if err == nil && found {
			err = progress.UnmarshalVT(data)
		}
		return err
	})
	if err != nil {
		return err
	}

	// Read, once per walk, how the walk treats linked chunks.
	mode, err := a.readLinkedCopy(ctx, so, progress.GetLinkedProofs())
	if err != nil {
		return err
	}

	// Retain the head and republish when this head already completed, unless
	// a full copy must fetch the linked chunks an earlier walk skipped.
	if progress.GetComplete() && progress.GetHead().EqualVT(head) && !mode.recheck {
		if err := holdAccountWorld(ctx, so.GetBlockStore(), head.GetRootRef()); err != nil {
			return err
		}
		state.publishCopyProgress(progress)
		return nil
	}

	// Mark the proofs before a walk that may count linked chunks as present,
	// so a later full copy knows to trust none of them. The mark stays across
	// heads, since the proofs do.
	progress = &AccountReplicaCopyState{
		ObjectId:     so.GetSharedObjectID(),
		Head:         head.CloneVT(),
		LinkedProofs: progress.GetLinkedProofs(),
	}
	if !mode.full && !progress.GetLinkedProofs() {
		progress.LinkedProofs = true
		if err := writeCopyProgress(ctx, local, progress); err != nil {
			return err
		}
	}

	// Copy the World and hold its root on success.
	state.publishCopyProgress(progress)
	err = a.copyAccountWorld(ctx, so, progress, absent, func() { state.publishCopyProgress(progress) }, mode)

	// Record the outcome in the progress state. A full copy that completed
	// rechecked every block, so no proof counts a linked chunk any more.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		progress.Error = err.Error()
	} else {
		progress.Complete = true
		if mode.full {
			progress.LinkedProofs = false
		}
	}

	// The block fence precedes this transaction, so a crash cannot retain
	// a completion claim whose data was still buffered.
	persistErr := writeCopyProgress(ctx, local, progress)
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

// writeCopyProgress stores the progress record.
func writeCopyProgress(ctx context.Context, local kvtx.Store, progress *AccountReplicaCopyState) error {
	openWrite := func(ctx context.Context) (kvtx.Tx, error) { return local.NewTransaction(ctx, true) }
	return kvtx.RunTransaction(ctx, true, openWrite, func(ctx context.Context, tx kvtx.Tx) error {
		data, err := progress.MarshalVT()
		if err != nil {
			return err
		}
		return tx.Set(ctx, []byte("account-replica-copy/progress"), data)
	})
}

// copyAccountWorld copies the World under the progress head and holds it
// under the account World root, publishing progress while blocks copy. A
// missing block does not stop the walk: the walk completes, keeps the proofs
// of every complete subtree, and returns a worldHolesError without moving the
// root. A block in absent is recorded as a hole without a read; the holes
// this walk finds become absent's set. mode sets how the walk treats the
// chunks a linked file serves (copyWorldGraph).
func (a *ProviderAccount) copyAccountWorld(
	ctx context.Context,
	so sobject.SharedObject,
	progress *AccountReplicaCopyState,
	absent *knownAbsent,
	changed func(),
	mode linkedCopy,
) error {
	// Count the copied blocks, publishing progress at most every 250ms.
	lastUpdate := time.Now()
	visited := func(_ *block.BlockRef, data []byte) {
		progress.Blocks++
		progress.Bytes += uint64(len(data))
		if time.Since(lastUpdate) >= 250*time.Millisecond {
			changed()
			lastUpdate = time.Now()
		}
	}

	// Read through the known-absent set, collecting the blocks no source
	// serves.
	gen := absent.generation()
	holes := &worldHolesError{}
	missing := func(ref *block.BlockRef) { holes.refs = append(holes.refs, ref.CloneVT()) }
	store := so.GetBlockStore()
	src := &absentSource{StoreOps: store, absent: absent}

	// Copy the World, then make its holes the known-absent set.
	if err := copyWorldGraph(ctx, store, src, progress.GetHead().GetRootRef(), visited, missing, mode); err != nil {
		return err
	}
	absent.replace(gen, holes.refs)
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
