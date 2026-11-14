package provider_local

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/world"
)

// soStateEntry is the mounted state of one shared object: what storage holds.
type soStateEntry struct {
	// id is the shared object id.
	id string
	// stateProm completes after loading the persisted state, once stateCtr
	// holds it.
	stateProm *promise.PromiseContainer[*sobject.SOState]
	// stateCtr publishes the committed state.
	stateCtr *ccontainer.CContainer[*sobject.SOState]
	// stamp identifies the stored version of the state in stateCtr, which
	// the next write requires storage still to hold, and selects the layout
	// it writes. It is set before stateProm completes, and then only under
	// writeMtx.
	stamp soStateStamp
	// writeMtx serializes the state writes while the entry is retained.
	writeMtx csync.Mutex
}

// newSOStateEntry constructs the entry of the state of id.
func newSOStateEntry(id string) *soStateEntry {
	return &soStateEntry{
		id:        id,
		stateProm: promise.NewPromiseContainer[*sobject.SOState](),
		stateCtr:  ccontainer.NewCContainer[*sobject.SOState](nil),
	}
}

// execute loads the committed state of sharedObjectID from objStore and holds
// the entry until ctx ends.
func (e *soStateEntry) execute(ctx context.Context, objStore object.ObjectStore, runs *SOStateRuns, sharedObjectID string) error {
	// Load the stored state after the open writes of this object, in the
	// layout StateLayoutEnv selects.
	val, stamp, err := e.load(ctx, objStore, runs)
	if err != nil {
		return err
	}

	// Publish it to waiters and watches, and hold the entry.
	e.stamp = stamp
	e.stateCtr.SetValue(val)
	e.stateProm.SetResult(val, nil)
	<-ctx.Done()
	return nil
}

// load reads and validates the stored state. It first takes runs.Writes alone
// and releases it at once, so it waits for the state writes of this object
// past their mount check, including those of the mount run before it, and for
// no other writer. A restart can start the load while such a write has yet to
// commit; the load then reads that write rather than the state under it. A
// write of the old run that takes the gate later finds its mount ended and
// writes nothing, and the new run's writes wait for this load. The read holds
// no lock, so it orders after no write transaction.
//
// A state stored in the other layout than StateLayoutEnv selects is converted
// in a write transaction that first checks storage still holds the stamp read;
// when it does not, another writer moved the state, so the load reads again.
func (e *soStateEntry) load(ctx context.Context, objStore object.ObjectStore, runs *SOStateRuns) (*sobject.SOState, soStateStamp, error) {
	// Wait for the writes of this object past their mount check.
	release, err := runs.Writes.Lock(ctx, true)
	if err != nil {
		return nil, soStateStamp{}, err
	}
	release()

	for {
		// Read and validate the state through the store transaction boundary.
		var val *sobject.SOState
		var stamp soStateStamp
		var found bool
		err := kvtx.RunTransaction(ctx, false,
			func(ctx context.Context) (kvtx.Tx, error) {
				return objStore.NewTransaction(ctx, false)
			},
			func(ctx context.Context, tx kvtx.Tx) error {
				var err error
				val, stamp, found, err = readSOState(ctx, tx, e.id)
				return err
			},
		)
		if err != nil {
			return nil, stamp, err
		}
		if !found {
			return nil, stamp, world.ErrObjectNotFound
		}
		if err := val.Validate(e.id); err != nil {
			return nil, stamp, err
		}
		if stamp.split == splitStateOn {
			return val, stamp, nil
		}

		// Convert it to the selected layout over the stamp read.
		var changed bool
		next := stamp
		err = kvtx.RunTransaction(ctx, true,
			func(ctx context.Context) (kvtx.Tx, error) {
				return objStore.NewTransaction(ctx, true)
			},
			func(ctx context.Context, tx kvtx.Tx) error {
				// Leave a state another writer moved since the read.
				stored, found, err := readSOStateStamp(ctx, tx, e.id)
				if err != nil {
					return err
				}
				if changed = !found || !stored.equal(stamp); changed {
					return nil
				}

				// Convert the state read.
				next, err = convertSOState(ctx, tx, e.id, val, stamp)
				return err
			},
		)
		if err != nil {
			return nil, stamp, err
		}
		if !changed {
			return val, next, nil
		}
	}
}

// commit publishes state, stored as stamp. The caller holds writeMtx.
func (e *soStateEntry) commit(state *sobject.SOState, stamp soStateStamp) {
	e.stamp = stamp
	e.stateCtr.SetValue(state)
	e.stateProm.SetResult(state, nil)
}
