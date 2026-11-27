package flowgraph_run

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// RecordActivationActor records the actor started for a running activation.
// Repeating the same key succeeds. Replacing the key is rejected. Create the
// actor and record it in the same write transaction so a crash cannot orphan
// it.
func RecordActivationActor(ctx context.Context, ws world.WorldState, runKey, activationID, actorKey string) error {
	// Find the activation before making any change.
	obj, run, _, err := readRun(ctx, ws, runKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	next := run.CloneVT()
	var activation *s4wave_flowgraph.FlowgraphActivation
	for _, candidate := range next.GetActivations() {
		if candidate.GetTaskKey() == activationID && !candidate.GetDone() {
			activation = candidate
		}
	}
	if activation == nil {
		return ErrActivationEnded
	}

	// Keep the first actor immutable and require a real actor object.
	if actorKey == "" {
		return errors.New("actor key is required")
	}
	if activation.GetActorKey() == actorKey {
		return nil
	}
	if activation.GetActorKey() != "" {
		return errors.New("activation already has an actor")
	}
	exists, err := ws.HasObject(ctx, actorKey)
	if err != nil {
		return err
	}
	if !exists {
		return world.ErrObjectNotFound
	}

	// Commit the actor receipt with the rest of the caller's transaction.
	activation.ActorKey = actorKey
	_, _, err = world.AccessObjectState(ctx, obj, true, func(cursor *block.Cursor) error {
		cursor.SetBlock(next, true)
		return nil
	})
	return err
}
