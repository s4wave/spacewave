package sobject_world_engine

import (
	"context"
	"time"

	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
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
type BackfillEngine interface {
	// SetBackfill chooses whether this device copies the installed World and
	// the retained roots into its local store in the background. The choice
	// persists on this device.
	SetBackfill(ctx context.Context, enabled bool) error
	// GetBackfill watches this device's backfill choice.
	GetBackfill() ccontainer.Watchable[bool]
}

// SetBackfill stores this device's backfill choice and starts or stops the
// copy.
func (e *soEngine) SetBackfill(ctx context.Context, enabled bool) error {
	// Serialize choices so the stored and the published choice agree.
	unlockWriteMtx, err := e.c.writeMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlockWriteMtx()

	// Store the choice, then publish it and start or stop the copy.
	if err := writeBackfill(ctx, e.so, enabled); err != nil {
		return err
	}
	e.backfillChoice.SetValue(enabled)
	e.backfill.SetState(enabled)
	return nil
}

// GetBackfill watches this device's backfill choice.
func (e *soEngine) GetBackfill() ccontainer.Watchable[bool] {
	return e.backfillChoice
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
// local store, then again after each World change. Completion proofs make each
// copy skip the subtrees an earlier copy finished.
func (e *soEngine) executeBackfill(ctx context.Context, _ bool) error {
	le := e.c.le
	for {
		// Take the wait before the targets, so a change during the copy is
		// not missed.
		var waitCh <-chan struct{}
		e.c.writeBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
		})
		roots, err := e.backfillRoots(ctx)
		if err != nil {
			return err
		}

		// Copy them.
		start := time.Now()
		if err := copyWorlds(ctx, e.so, backfillProofStoreID, roots); err != nil {
			return err
		}
		le.WithField("dur", time.Since(start).String()).Debug("backfilled the World")

		// Wait for the next change.
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// backfillRoots returns the installed World root and the retained roots.
func (e *soEngine) backfillRoots(ctx context.Context) ([]*block.BlockRef, error) {
	// Read them under the writer lock.
	unlockWriteMtx, err := e.c.writeMtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlockWriteMtx()

	// Collect the head, then the retained roots.
	roots := []*block.BlockRef{e.retained.GetRootRef()}
	for _, root := range e.retainedRoots {
		roots = append(roots, root.GetRootRef())
	}
	return roots, nil
}

// _ is a type assertion
var _ BackfillEngine = (*soEngine)(nil)
