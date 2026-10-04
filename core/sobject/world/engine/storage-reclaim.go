package sobject_world_engine

import (
	"bytes"
	"context"
	"encoding/hex"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
)

// storageReclaimDelay is the wait after a World change before a storage
// reclaim pass, so the volume GC deletes the blocks the change released from
// the local store first.
const storageReclaimDelay = 5 * time.Minute

// storageReclaimInterval is the minimum time between storage reclaim requests.
// A pass reads the key index of every packfile on the storage backend.
const storageReclaimInterval = time.Hour

// reclaimFenceTimeout is the longest a pass waits for every roster device to
// answer its fence. A device offline that long fails the pass, and the next
// request tries again.
const reclaimFenceTimeout = 10 * time.Minute

// reclaimFenceRootName names the local root holding the Worlds above the
// checkpoint while a pass judges which blocks are live.
const reclaimFenceRootName = "reclaim-fence"

// reclaimFenceProofStoreID is the local state store holding the completion
// proofs of the Worlds the fence holds.
const reclaimFenceProofStoreID = "reclaim-fence-retention"

// errReclaimFenceUnanswered is returned when a roster device did not answer
// the fence within reclaimFenceTimeout.
var errReclaimFenceUnanswered = errors.New("a roster device did not answer the storage reclaim fence")

// errReclaimWorldsNotHeld is returned when the replay does not hold the World
// after every operation above the checkpoint, as after a restart.
var errReclaimWorldsNotHeld = errors.New("replay does not hold every World above the checkpoint")

// executeStorageReclaim asks the block store for a storage reclaim pass
// storageReclaimDelay after startup and after each World change, and at the
// time the block store says the next pass comes due, so an idle Space still
// runs its last pass. Requests are at least storageReclaimInterval apart. The
// block store decides whether a pass pays for itself.
func (c *Controller) executeStorageReclaim(ctx context.Context, e *soEngine) error {
	// Track the pending request and the time of the previous one.
	var reclaimTimer *time.Timer
	var reclaimAt, lastReclaim time.Time
	defer func() {
		if reclaimTimer != nil {
			reclaimTimer.Stop()
		}
	}()

	// Arm the request for at, unless an earlier one is pending.
	schedule := func(at time.Time) {
		if earliest := lastReclaim.Add(storageReclaimInterval); at.Before(earliest) {
			at = earliest
		}
		if reclaimTimer != nil {
			if !at.Before(reclaimAt) {
				return
			}
			reclaimTimer.Stop()
		}
		reclaimAt, reclaimTimer = at, time.NewTimer(time.Until(at))
	}
	schedule(time.Now().Add(storageReclaimDelay))

	// Schedule a request after each change, and send it when it is due. Hold
	// each wait channel until it fires, so a change during a pass is not
	// missed.
	var waitCh <-chan struct{}
	for {
		if waitCh == nil {
			c.writeBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
				waitCh = getWaitCh()
			})
		}
		var reclaimCh <-chan time.Time
		if reclaimTimer != nil {
			reclaimCh = reclaimTimer.C
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
			waitCh = nil
			schedule(time.Now().Add(storageReclaimDelay))
		case <-reclaimCh:
			reclaimTimer = nil
			lastReclaim = time.Now()
			next, err := e.reclaimStorage(ctx)
			if err != nil {
				return err
			}
			if !next.IsZero() {
				schedule(next)
			}
		}
	}
}

// notifyWrite wakes the storage reclaim routine after a World change.
func (c *Controller) notifyWrite() {
	c.writeBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
}

// reclaimStorage drops the blocks the local store no longer holds from the
// Space's storage backend. Only the checkpointer runs a pass, since its fence
// waits on the stable point. Returns the time the next pass comes due without
// further changes, or zero. A failed pass is logged, and the next change or
// due time schedules another.
func (e *soEngine) reclaimStorage(ctx context.Context) (time.Time, error) {
	// Only the checkpointer reclaims.
	snap, err := e.so.GetSharedObjectState(ctx)
	if err != nil {
		return time.Time{}, err
	}
	cfg, err := snap.GetConfig(ctx)
	if err != nil {
		return time.Time{}, err
	}
	if cfg.Checkpointer() != e.so.GetPeerID().String() {
		return time.Time{}, nil
	}

	// Run the pass, then release the Worlds the fence held.
	store := e.so.GetBlockStore()
	next, err := store.ReclaimStorage(ctx, e.fenceStorageReclaim)
	if ctx.Err() != nil {
		return time.Time{}, ctx.Err()
	}
	if relErr := block.SetRetainedRoot(ctx, store, reclaimFenceRootName, nil); err == nil {
		err = relErr
	}
	if err != nil {
		e.c.le.WithError(err).Warn("storage reclaim pass failed")
	}
	return next, nil
}

// fenceStorageReclaim makes the local store hold every block a roster device
// may still reference, so a pass drops only blocks unreachable from the World
// at the stable point, from every operation above it and from the retained
// roots.
//
// It writes an acknowledgment, which asks every roster device to answer after
// the operations it has already started, and waits until the stable point
// covers it. Every operation a roster device started before it saw the fence
// is then placed here, and any later one uploads its blocks after the backend
// listed the packfiles the pass judges. It then copies those Worlds complete,
// since this device holds only the blocks it wrote or read, and holds the
// World after each operation above the checkpoint. The names of the
// checkpoint's World and of the retained roots already hold the rest.
func (e *soEngine) fenceStorageReclaim(ctx context.Context) error {
	// Write the fence after this device's started operations.
	h, nonce, err := e.writeFence(ctx)
	if err != nil {
		return err
	}

	// Wait for every roster device to build on it.
	if err := e.waitStable(ctx, h, nonce); err != nil {
		return err
	}

	// Copy every live World, then hold the Worlds above the checkpoint.
	base, roots, retained, err := e.liveWorlds(ctx)
	if err != nil {
		return err
	}
	copies := []*block.BlockRef{base}
	for _, root := range slices.Concat(roots, retained) {
		copies = append(copies, root.GetRootRef())
	}
	if err := copyWorlds(ctx, e.so, reclaimFenceProofStoreID, copies); err != nil {
		return err
	}
	return holdRootSet(ctx, e.so, reclaimFenceRootName, roots)
}

// writeFence queues an acknowledgment after every write transaction this
// device has started and returns its hash and nonce.
func (e *soEngine) writeFence(ctx context.Context) ([]byte, uint64, error) {
	// Queue it under the writer lock.
	unlockWriteMtx, err := e.c.writeMtx.Lock(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer unlockWriteMtx()
	localID, err := e.so.QueueOperation(ctx, nil)
	if err != nil {
		return nil, 0, err
	}

	// Find it in the operation set.
	snap, err := e.so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, 0, err
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return nil, 0, err
	}
	h := set.Find(e.so.GetPeerID().String(), localID)
	if h == nil {
		return nil, 0, errors.New("queued fence is missing from the operation set")
	}
	return h, set.Get(h).GetNonce(), nil
}

// waitStable waits until the stable point or the checkpoint covers the local
// operation h with nonce, for at most reclaimFenceTimeout.
func (e *soEngine) waitStable(ctx context.Context, h []byte, nonce uint64) error {
	// Watch the state until the deadline.
	ctx, cancel := context.WithTimeoutCause(ctx, reclaimFenceTimeout, errReclaimFenceUnanswered)
	defer cancel()
	ctr, rel, err := e.so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer rel()
	self := e.so.GetPeerID().String()

	// Check each state until the fence is stable.
	var snap sobject.SharedObjectStateSnapshot
	for {
		// Wait for the next state.
		snap, err = ctr.WaitValueChange(ctx, snap, nil)
		if err != nil {
			return context.Cause(ctx)
		}
		if snap == nil {
			continue
		}

		// Finish once the fence is stable.
		cfg, err := snap.GetConfig(ctx)
		if err != nil {
			return err
		}
		set, err := snap.GetOperationSet(ctx)
		if err != nil {
			return err
		}
		stable := set.StablePoint(cfg.TrimRoster())
		if set.Covers(self, nonce) || slices.ContainsFunc(stable, func(s []byte) bool { return bytes.Equal(s, h) }) {
			return nil
		}
	}
}

// liveWorlds replays the current operation set and returns the checkpoint's
// World root, the World root after each operation above the checkpoint, named
// by the operation's hash, and the retained roots. Returns
// errReclaimWorldsNotHeld when the replay restored positions without their
// Worlds.
func (e *soEngine) liveWorlds(ctx context.Context) (*block.BlockRef, []*RetainedRoot, []*RetainedRoot, error) {
	// Replay the current state under the writer lock.
	unlockWriteMtx, err := e.c.writeMtx.Lock(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	defer unlockWriteMtx()
	snap, err := e.so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	if _, err := e.advance(ctx, snap, nil); err != nil {
		return nil, nil, nil, err
	}

	// Collect each distinct root after the base.
	var roots []*RetainedRoot
	base := e.replay.base.GetHeadRef().GetRootRef()
	prev := base
	for _, pos := range e.replay.positions {
		if pos.state == nil {
			return nil, nil, nil, errReclaimWorldsNotHeld
		}
		ref := pos.state.GetHeadRef().GetRootRef()
		if ref.GetEmpty() || ref.EqualVT(prev) {
			continue
		}
		roots = append(roots, &RetainedRoot{Name: hex.EncodeToString(pos.outcome.hash), RootRef: ref})
		prev = ref
	}
	return base, roots, e.retainedRoots, nil
}
