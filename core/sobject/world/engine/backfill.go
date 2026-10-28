package sobject_world_engine

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/sirupsen/logrus"
)

// backfillStoreID is the local state store holding this device's backfill
// choice for the World.
const backfillStoreID = "world-backfill"

// backfillKey is the key of the backfill choice in its store. It is present
// when backfill is on.
var backfillKey = []byte("enabled")

// backfillProofStoreID is the local state store holding the completion proofs
// of the backfilled Worlds.
const backfillProofStoreID = "accepted-world-retention"

// backfillBackoff spaces the retries of a failed backfill copy, such as one
// whose peers went away.
var backfillBackoff = &backoff.Backoff{
	BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
	Exponential: &backoff.Exponential{
		InitialInterval: 1000,
		MaxInterval:     60000,
		Multiplier:      2,
	},
}

// BackfillEngine is a World engine whose device can keep a complete local copy
// of the World. Without backfill the device reads blocks on demand and keeps
// the blocks it wrote or read while the World still references them.
//
// The served engine implements it, not the BusEngine that world.StartEngine
// returns: assert it inside world.AccessEngine.
type BackfillEngine interface {
	// SetBackfill chooses whether this device copies the installed World and
	// the retained roots into its local store in the background. The choice
	// persists on this device.
	SetBackfill(ctx context.Context, enabled bool) error
	// GetBackfill watches this device's backfill choice.
	GetBackfill() ccontainer.Watchable[bool]
	// GetBackfillState watches the outcome of the last finished copy, nil
	// while backfill is off or before its first copy finishes.
	GetBackfillState() ccontainer.Watchable[*BackfillState]
	// RecheckBackfill restarts the copy so it walks every block of the
	// covered roots, trusting no completion proof, and fetches each one the
	// store lacks. A false proof, such as one an older build recorded, hides
	// the blocks under it from every other copy. It returns once the
	// recheck starts; GetBackfillState reports its outcome.
	RecheckBackfill(ctx context.Context) error
	// GetHead returns the installed World root and its seqno, read once the
	// installed World is durable. A copy that covers the same root and seqno
	// holds the same World.
	GetHead(ctx context.Context) (*block.BlockRef, uint64, error)
}

// IsComplete reports whether the local store held every block of the covered
// roots.
func (s *BackfillState) IsComplete() bool {
	return s != nil && s.GetHoles() == 0
}

// SetBackfill stores this device's backfill choice and starts or stops the
// copy.
func (e *soEngine) SetBackfill(ctx context.Context, enabled bool) error {
	// Serialize choices so the stored and the published choice agree.
	unlockWriteMtx, err := e.c.writeMtx.Lock(ctx, "set-backfill")
	if err != nil {
		return err
	}
	defer unlockWriteMtx()

	// Store the choice, then publish it and start or stop the copy. Stopping
	// cancels the copy before it can publish, so its outcome clears for good.
	if err := writeBackfill(ctx, e.so, enabled); err != nil {
		return err
	}
	e.backfillChoice.SetValue(enabled)
	e.backfill.SetState(enabled)
	if !enabled {
		e.backfillState.SetValue(nil)
	}
	return nil
}

// GetBackfill watches this device's backfill choice.
func (e *soEngine) GetBackfill() ccontainer.Watchable[bool] {
	return e.backfillChoice
}

// GetBackfillState watches the outcome of the last finished copy.
func (e *soEngine) GetBackfillState() ccontainer.Watchable[*BackfillState] {
	return e.backfillState
}

// RecheckBackfill restarts the copy as a recheck.
func (e *soEngine) RecheckBackfill(ctx context.Context) error {
	// Serialize with the choice, so a recheck never runs with backfill off.
	unlockWriteMtx, err := e.c.writeMtx.Lock(ctx, "recheck-backfill")
	if err != nil {
		return err
	}
	defer unlockWriteMtx()
	if !e.backfillChoice.GetValue() {
		return errors.New("backfill is off")
	}

	// Clear the last outcome so a waiter sees the recheck's, then restart.
	e.backfillRecheck.Store(true)
	e.backfillState.SetValue(nil)
	e.backfill.RestartRoutine()
	return nil
}

// readBackfill reads this device's backfill choice for the World of so.
func readBackfill(ctx context.Context, so sobject.SharedObject) (bool, error) {
	// Open the choice's store.
	store, release, err := so.AccessLocalStateStore(ctx, backfillStoreID, nil)
	if err != nil {
		return false, err
	}
	defer release()

	// Read the choice.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return false, err
	}
	defer tx.Discard()
	_, found, err := tx.Get(ctx, backfillKey)
	return found, err
}

// writeBackfill stores this device's backfill choice for the World of so.
func writeBackfill(ctx context.Context, so sobject.SharedObject, enabled bool) error {
	// Open the choice's store.
	store, release, err := so.AccessLocalStateStore(ctx, backfillStoreID, nil)
	if err != nil {
		return err
	}
	defer release()

	// Set or delete the key.
	return kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		if enabled {
			return tx.Set(ctx, backfillKey, []byte{1})
		}
		return tx.Delete(ctx, backfillKey)
	})
}

// executeBackfill copies the installed World and the retained roots into the
// local store, then again after each World change, and publishes the outcome of
// each copy. Completion proofs make each copy skip the subtrees an earlier copy
// finished.
func (e *soEngine) executeBackfill(ctx context.Context, _ bool) error {
	le := e.c.le
	recheck := e.backfillRecheck.Load()
	for {
		// Take the wait before the targets, so a change during the copy is
		// not missed.
		var waitCh <-chan struct{}
		e.c.writeBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
		})
		roots, seqno, err := e.backfillRoots(ctx)
		if err != nil {
			return err
		}

		// Copy them, logging each block no peer holds and continuing past it.
		start := time.Now()
		var holes uint32
		missing := func(ref *block.BlockRef) {
			holes++
			le.WithField("block", ref.MarshalString()).Warn("backfill found a block no peer holds")
		}
		opts := &RetainOptions{Missing: missing, Recheck: recheck}
		if recheck {
			opts.Visited = recheckProgress(le, start)
		}
		if err := copyWorlds(ctx, e.so, backfillProofStoreID, roots, opts); err != nil {
			return err
		}
		fields := logrus.Fields{"dur": time.Since(start).String(), "holes": holes}
		if recheck {
			// Later copies trust the proofs this one recorded.
			e.backfillRecheck.Store(false)
			recheck = false
			le.WithFields(fields).Info("rechecked the backfilled World")
		} else {
			le.WithFields(fields).Debug("backfilled the World")
		}
		if err := e.publishBackfill(ctx, &BackfillState{Root: roots[0], Holes: holes, Seqno: seqno}); err != nil {
			return err
		}

		// Wait for the next change.
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// recheckProgressInterval is how often a recheck logs its progress.
const recheckProgressInterval = time.Minute

// recheckProgress returns a Visited callback that counts the blocks a recheck
// walks and logs the count and bytes about once per recheckProgressInterval.
// CopyGraph calls Visited from one goroutine.
func recheckProgress(le *logrus.Entry, start time.Time) func(*block.BlockRef, []byte) {
	var blocks, size uint64
	next := start.Add(recheckProgressInterval)
	return func(_ *block.BlockRef, data []byte) {
		blocks++
		size += uint64(len(data))
		if now := time.Now(); now.After(next) {
			next = now.Add(recheckProgressInterval)
			le.WithFields(logrus.Fields{
				"blocks": blocks,
				"bytes":  size,
				"dur":    now.Sub(start).Round(time.Second).String(),
			}).Info("rechecking the backfilled World")
		}
	}
}

// publishBackfill publishes the outcome of a copy under the writer lock, unless
// SetBackfill stopped the copy first.
func (e *soEngine) publishBackfill(ctx context.Context, state *BackfillState) error {
	// Take the lock SetBackfill holds, then check the copy is still wanted.
	unlockWriteMtx, err := e.c.writeMtx.Lock(ctx, "publish-backfill")
	if err != nil {
		return err
	}
	defer unlockWriteMtx()
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	e.backfillState.SetValue(state)
	return nil
}

// backfillRoots returns the installed World root, then the retained roots, and
// the World seqno of the installed root.
func (e *soEngine) backfillRoots(ctx context.Context) ([]*block.BlockRef, uint64, error) {
	// Read them under the writer lock once the installed World is durable,
	// since the copy reads only the store.
	unlockWriteMtx, err := e.c.lockSettled(ctx, "backfill")
	if err != nil {
		return nil, 0, err
	}
	defer unlockWriteMtx()
	seqno, err := e.bengine.GetSeqno(ctx)
	if err != nil {
		return nil, 0, err
	}

	// Collect the head, then the retained roots.
	roots := []*block.BlockRef{e.retained.GetRootRef()}
	for _, root := range e.retainedRoots {
		roots = append(roots, root.GetRootRef())
	}
	return roots, seqno, nil
}

// GetHead returns the installed World root and its seqno.
func (e *soEngine) GetHead(ctx context.Context) (*block.BlockRef, uint64, error) {
	roots, seqno, err := e.backfillRoots(ctx)
	if err != nil {
		return nil, 0, err
	}
	return roots[0], seqno, nil
}

// _ is a type assertion
var _ BackfillEngine = (*soEngine)(nil)

// ReplayWorldsEngine lists the World roots this device still names.
type ReplayWorldsEngine interface {
	// ReplayWorlds returns, newest first and each once, the World after each
	// replayed operation, the replay base, and the retained roots. A repair
	// reads an older copy of a damaged part of the World from them.
	ReplayWorlds(ctx context.Context) ([]*block.BlockRef, error)
}

// ReplayWorlds returns the World roots of the replay and the retained roots,
// newest first.
func (e *soEngine) ReplayWorlds(ctx context.Context) ([]*block.BlockRef, error) {
	// Read them under the writer lock, which guards the replay.
	unlockWriteMtx, err := e.c.lockSettled(ctx, "replay-worlds")
	if err != nil {
		return nil, err
	}
	defer unlockWriteMtx()

	// Collect each position's World from the newest, then the base and the
	// retained roots.
	var roots []*block.BlockRef
	add := func(ref *block.BlockRef) {
		if ref.GetEmpty() || slices.ContainsFunc(roots, ref.EqualsRef) {
			return
		}
		roots = append(roots, ref.Clone())
	}
	for i := len(e.replay.positions) - 1; i >= 0; i-- {
		add(e.replay.positions[i].state.GetHeadRef().GetRootRef())
	}
	add(e.replay.base.GetHeadRef().GetRootRef())
	for _, root := range e.retainedRoots {
		add(root.GetRootRef())
	}
	return roots, nil
}

// _ is a type assertion
var _ ReplayWorldsEngine = (*soEngine)(nil)
