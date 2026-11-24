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
	// layout stores the state: the selected layout, which the mount's state
	// load converted the stored state to.
	layout soStateLayout
	// stateProm completes after loading the persisted state, once stateCtr
	// holds it.
	stateProm *promise.PromiseContainer[*sobject.SOState]
	// stateCtr publishes the committed state.
	stateCtr *ccontainer.CContainer[*sobject.SOState]
	// token is the bytes of the layout's token key that store the state in
	// stateCtr, which the next write requires storage still to hold. It is
	// set before stateProm completes, and then only under writeMtx.
	token []byte
	// writeMtx serializes the state writes while the entry is retained.
	writeMtx csync.Mutex
}

// newSOStateEntry constructs the entry of the state stored in layout.
func newSOStateEntry(layout soStateLayout) *soStateEntry {
	return &soStateEntry{
		layout:    layout,
		stateProm: promise.NewPromiseContainer[*sobject.SOState](),
		stateCtr:  ccontainer.NewCContainer[*sobject.SOState](nil),
	}
}

// execute loads the committed state of sharedObjectID from objStore and holds
// the entry until ctx ends.
func (e *soStateEntry) execute(ctx context.Context, objStore object.ObjectStore, runs *SOStateRuns, sharedObjectID string) error {
	// Load the stored state after the open writes of this object.
	val, token, found, err := e.load(ctx, objStore, runs)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	// Validate the persisted state before exposing it to consumers.
	if err := val.Validate(sharedObjectID); err != nil {
		return err
	}

	// Publish it to waiters and watches, and hold the entry.
	e.token = token
	e.stateCtr.SetValue(val)
	e.stateProm.SetResult(val, nil)
	<-ctx.Done()
	return nil
}

// load reads the stored state. It first takes runs.Writes alone and
// releases it at once, so it waits for the state writes of this object past
// their mount check, including those of the mount run before it,
// and for no other writer. A restart can start the load while such a write
// has yet to commit; the load then reads that write rather than the state
// under it. A write of the old run that takes the gate later finds its mount
// ended and writes nothing, and the new run's writes wait for this load. The
// read holds no lock, so it orders after no write transaction.
func (e *soStateEntry) load(ctx context.Context, objStore object.ObjectStore, runs *SOStateRuns) (state *sobject.SOState, token []byte, found bool, err error) {
	// Wait for the writes of this object past their mount check.
	release, err := runs.Writes.Lock(ctx, true)
	if err != nil {
		return nil, nil, false, err
	}
	release()

	// Read the state through the store transaction boundary.
	err = kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			var err error
			state, token, found, err = e.layout.load(ctx, tx)
			return err
		},
	)
	return state, token, found, err
}

// commit publishes state, stored under token. The caller holds writeMtx.
func (e *soStateEntry) commit(state *sobject.SOState, token []byte) {
	e.token = token
	e.stateCtr.SetValue(state)
	e.stateProm.SetResult(state, nil)
}
