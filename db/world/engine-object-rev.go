package world

import "context"

// WaitObjectRevBySeqno waits for an object revision by rereading the object
// after every World revision. Engines without a key-scoped change watch use it;
// each World commit costs every waiter one read transaction.
func WaitObjectRevBySeqno(
	ctx context.Context,
	e Engine,
	key string,
	rev uint64,
	ignoreNotFound bool,
) (uint64, error) {
	// Reuse the World operation retry policy for each reread.
	ws := &engineWorldState{e: e}
	for {
		// Read the object revision together with the World revision it was read at.
		var seqno, currRev uint64
		var found bool
		err := ws.performOp(ctx, false, func(tx Tx) error {
			// Read the World revision the next wait starts after.
			var err error
			seqno, err = tx.GetSeqno(ctx)
			if err != nil {
				return err
			}

			// Read the object revision from the same snapshot.
			currRev, found, err = GetObjectRev(ctx, tx, key)
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
			return 0, ErrObjectNotFound
		}

		// Wait for any later World revision, then reread.
		if _, err := e.WaitSeqno(ctx, seqno+1); err != nil {
			return 0, err
		}
	}
}

// GetObjectRev returns the revision of the object at key and whether it exists.
func GetObjectRev(ctx context.Context, ws WorldState, key string) (uint64, bool, error) {
	// Look up the object, releasing its state after the read.
	obj, found, err := ws.GetObject(ctx, key)
	defer ReleaseObjectState(obj)
	if err != nil || !found {
		return 0, false, err
	}

	// Read the revision from the object's root reference.
	_, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		return 0, false, err
	}
	return rev, true, nil
}
