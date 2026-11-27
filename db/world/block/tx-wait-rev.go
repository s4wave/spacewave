package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// WaitObjectRev waits until the object at key reaches rev and returns its
// revision. It returns ErrObjectNotFound if the object does not exist, unless
// ignoreNotFound is set, in which case it waits for the object to appear.
//
// Only a change that references key wakes the wait, so a change to another
// object costs it nothing, and a create of a missing key wakes it although a
// run of same-type changes leaves the seqno where it was. A read snapshot
// cannot advance: its wait ends with ctx or the snapshot's discard.
func (t *Tx) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	for {
		// Read the revision under the read lock, which a writer's change holds
		// the write lock against.
		var woken <-chan struct{}
		var currRev uint64
		var found bool
		err := t.lockedLive(ctx, func() error {
			// Take the wake first, so a later change closes it.
			woken = t.state.objectWake(key)

			// Read the object's revision, if it exists.
			obj, ok, err := t.state.getObject(ctx, key)
			if err != nil || !ok {
				return err
			}
			found = true
			_, currRev, err = obj.GetRootRef(ctx)
			return err
		})
		if err != nil {
			return 0, err
		}

		// Return a satisfied revision or a missing object the caller rejects.
		if found && currRev >= rev {
			return currRev, nil
		}
		if !found && !ignoreNotFound {
			return 0, world.ErrObjectNotFound
		}

		// Wait for the next change to key.
		if err := t.state.waitRev(ctx, woken); err != nil {
			return 0, err
		}
	}
}

// WaitObjectRev waits until the object at key reaches rev, with the contract
// of Tx.WaitObjectRev, on the transaction's pending writer or read snapshot.
func (e *EngineTx) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	return performOpValue(ctx, e, func(tx *Tx) (uint64, error) {
		return tx.WaitObjectRev(ctx, key, rev, ignoreNotFound)
	})
}

// _ is a type assertion
var (
	_ world.ObjectRevWaiter = (*Tx)(nil)
	_ world.ObjectRevWaiter = (*EngineTx)(nil)
)
