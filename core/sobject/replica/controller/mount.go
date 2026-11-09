package sobject_replica_controller

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/conc"
	"github.com/pkg/errors"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/world"
)

// mount is the replicated Space mounted on this Device while it participates.
type mount struct {
	// so is the mounted SharedObject.
	so *provider_local.SharedObject
	// state watches the SharedObject state.
	state ccontainer.Watchable[sobject.SharedObjectStateSnapshot]
	// engine is the Space's World engine.
	engine world.Engine
	// release stops the watches, then releases the state and the Space body.
	release func()
}

// openMount mounts the Space body of ref and its SharedObject state, and wakes
// the node on each change of either or of the copy until released.
func openMount(ctx context.Context, r *reconciler, ref *sobject.SharedObjectRef) (*mount, error) {
	// Mount the Space body, which runs its World engine.
	body, bodyRef, err := space.ExMountSpaceSoBody(ctx, r.c.b, ref, false, nil)
	if err != nil {
		return nil, err
	}
	so, ok := body.GetSharedObject().(*provider_local.SharedObject)
	if !ok {
		bodyRef.Release()
		return nil, errors.New("the replicated SharedObject is not held by the local provider")
	}

	// Watch the SharedObject state.
	state, releaseState, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		bodyRef.Release()
		return nil, err
	}
	m := &mount{so: so, state: state, engine: body.GetSharedObjectBody().GetWorldEngine()}

	// Wake the node on each change of the state or the copy.
	watchCtx, stopWatch := context.WithCancel(ctx)
	initial := state.GetValue()
	watches := conc.NewConcurrentQueue(0,
		func() { wakeOnChange(watchCtx, state, initial, r.c.wake) },
		func() { m.wakeOnBackfill(watchCtx, r.c.wake) },
	)

	// Release stops the watches before the state and the body they read.
	m.release = func() {
		stopWatch()
		_ = watches.WaitIdle(context.Background(), nil)
		releaseState()
		bodyRef.Release()
	}
	return m, nil
}

// accessBackfill calls cb with the backfill engine serving the Space's World.
func (m *mount) accessBackfill(ctx context.Context, cb func(be sobject_world_engine.BackfillEngine) error) error {
	return world.AccessEngine(ctx, m.engine, func(_ context.Context, eng world.Engine) error {
		be, ok := eng.(sobject_world_engine.BackfillEngine)
		if !ok {
			return errors.New("the replicated World cannot be backfilled")
		}
		return cb(be)
	})
}

// wakeOnBackfill calls wake each time the engine serving the World, its
// backfill choice or its copy's outcome changes, until ctx ends.
func (m *mount) wakeOnBackfill(ctx context.Context, wake func()) {
	_ = world.AccessEngine(ctx, m.engine, func(ctx context.Context, eng world.Engine) error {
		// Require the backfill engine.
		be, ok := eng.(sobject_world_engine.BackfillEngine)
		if !ok {
			return errors.New("the replicated World cannot be backfilled")
		}

		// Wake for this engine, then for each change of its choice or outcome
		// until the engine changes or ctx ends.
		choice, outcome := be.GetBackfill(), be.GetBackfillState()
		initialChoice, initialOutcome := choice.GetValue(), outcome.GetValue()
		wake()
		choiceDone := make(chan struct{})
		go func() {
			defer close(choiceDone)
			wakeOnChange(ctx, choice, initialChoice, wake)
		}()
		wakeOnChange(ctx, outcome, initialOutcome, wake)
		<-choiceDone
		return nil
	})
}
