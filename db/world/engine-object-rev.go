package world

import (
	"context"

	"github.com/pkg/errors"
)

// ObjectRevWaiter is a WorldState that waits for object revisions through a
// key-scoped watch instead of rereading the object after every World commit.
type ObjectRevWaiter interface {
	// WaitObjectRev waits until the object at key reaches rev and returns its
	// revision, with the contract of Engine.WaitObjectRev.
	WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error)
}

// WaitObjectRev waits until the object at key reaches rev on ws and returns its
// revision. It returns ErrObjectNotFound if the object does not exist, unless
// ignoreNotFound is set, in which case it waits for the object to appear. An
// ObjectRevWaiter waits through its watch. Any other WorldState waits on the
// object's own WaitRev, and rereads a missing object after each World commit.
func WaitObjectRev(ctx context.Context, ws WorldState, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	if waiter, ok := ws.(ObjectRevWaiter); ok {
		return waiter.WaitObjectRev(ctx, key, rev, ignoreNotFound)
	}
	for {
		// Read the seqno before the object, so a creation after the read
		// ends the wait below.
		seqno, err := ws.GetSeqno(ctx)
		if err != nil {
			return 0, err
		}
		obj, found, err := ws.GetObject(ctx, key)
		if err != nil {
			ReleaseObjectState(obj)
			return 0, err
		}

		// Wait on an existing object through its handle.
		if found {
			currRev, err := obj.WaitRev(ctx, rev, ignoreNotFound)
			ReleaseObjectState(obj)
			return currRev, err
		}
		if !ignoreNotFound {
			return 0, ErrObjectNotFound
		}

		// Wait for a later commit, which may create the object.
		if _, err := ws.WaitSeqno(ctx, seqno+1); err != nil {
			return 0, err
		}
	}
}

// WaitObjectChange waits until an object differs from the root a reader saw in
// seen: a later revision, its deletion, or, for an object seen missing, its
// creation. A view reads seen with GetObjectRootRefsBatch before it reads the
// objects, so a change after the read ends the wait. An object deleted and
// created again at its seen revision before the wait begins is not a change to
// the wait; a caller that waits long after its read compares the roots first.
// With no objects it waits until ctx ends.
func WaitObjectChange(ctx context.Context, ws WorldState, seen ...*ObjectRootRef) error {
	// Wait on one object directly.
	if len(seen) == 1 {
		return waitObjectRootChange(ctx, ws, seen[0])
	}

	// Wait on every object at once.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, len(seen))
	for _, ref := range seen {
		go func() {
			done <- waitObjectRootChange(ctx, ws, ref)
		}()
	}

	// Take the first change, or the end of ctx when there are no objects.
	var err error
	pending := len(seen)
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case err = <-done:
		pending--
	}

	// Cancel the other waits and let them finish before returning.
	cancel()
	for range pending {
		<-done
	}
	return err
}

// waitObjectRootChange waits until the object differs from ref.
func waitObjectRootChange(ctx context.Context, ws WorldState, ref *ObjectRootRef) error {
	if !ref.Exists {
		_, err := WaitObjectRev(ctx, ws, ref.ObjectKey, 0, true)
		return err
	}
	_, err := WaitObjectRev(ctx, ws, ref.ObjectKey, ref.Rev+1, false)
	if errors.Is(err, ErrObjectNotFound) {
		return nil
	}
	return err
}

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
