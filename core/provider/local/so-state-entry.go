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
	// key addresses the serialized SOState in the object store.
	key []byte
	// stateProm completes after loading the persisted state, once stateCtr
	// holds it.
	stateProm *promise.PromiseContainer[*sobject.SOState]
	// stateCtr publishes the committed state.
	stateCtr *ccontainer.CContainer[*sobject.SOState]
	// writeMtx serializes the state writes while the entry is retained.
	writeMtx csync.Mutex
}

// newSOStateEntry constructs the entry of the state at key.
func newSOStateEntry(key []byte) *soStateEntry {
	return &soStateEntry{
		key:       key,
		stateProm: promise.NewPromiseContainer[*sobject.SOState](),
		stateCtr:  ccontainer.NewCContainer[*sobject.SOState](nil),
	}
}

// execute loads the committed state of sharedObjectID from objStore and holds
// the entry until ctx ends.
func (e *soStateEntry) execute(ctx context.Context, objStore object.ObjectStore, sharedObjectID string) error {
	// Read the state through the store transaction boundary.
	var data []byte
	var found bool
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			var err error
			data, found, err = tx.Get(ctx, e.key)
			return err
		},
	)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	// Validate the persisted state before exposing it to consumers.
	val := &sobject.SOState{}
	if err := val.UnmarshalVT(data); err != nil {
		return err
	}
	if err := val.Validate(sharedObjectID); err != nil {
		return err
	}

	// Publish it to waiters and watches, and hold the entry.
	e.stateCtr.SetValue(val)
	e.stateProm.SetResult(val, nil)
	<-ctx.Done()
	return nil
}

// commit publishes state, written to storage. The caller holds writeMtx.
func (e *soStateEntry) commit(state *sobject.SOState) {
	e.stateCtr.SetValue(state)
	e.stateProm.SetResult(state, nil)
}
