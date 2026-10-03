package forge_task

import (
	"context"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// PredObjectRetired records that a Task or one of its attempts needs no further
// worker demand once complete. The self edge preserves this intent across restarts.
const PredObjectRetired = quad.IRI("forge/retired")

// RetireTask ends worker demand for a one-shot Task and all its Passes and
// Executions. Complete objects lose their keypair links immediately; unfinished
// objects retain demand until their worker observes completion. Bodies, results,
// peer authorization and history edges remain intact. The caller owns the write
// transaction. A nonempty sender must be the Task's assigned peer, as in cluster
// assignment; an empty sender uses the caller's trusted World write authority.
func RetireTask(ctx context.Context, ws world.WorldState, taskKey string, sender peer.ID) error {
	// Require retirement to come from the Task's assigned peer.
	task, err := LookupTaskBody(ctx, ws, taskKey)
	if err != nil {
		return err
	}
	if sender != "" && sender.String() != task.GetPeerId() {
		return errors.New("retirement sender does not match task peer")
	}

	// Collect only this Task's attempts, leaving its Job and siblings live.
	passes, err := ListTaskPasses(ctx, ws, taskKey)
	if err != nil {
		return err
	}
	executions, err := forge_pass.ListPassExecutions(ctx, ws, passes...)
	if err != nil {
		return err
	}
	keys := append([]string{taskKey}, passes...)
	keys = append(keys, executions...)

	// Persist retirement before detaching objects that already completed.
	for _, key := range keys {
		if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(key, PredObjectRetired.String(), key, "")); err != nil {
			return err
		}
		if _, err := ReconcileRetiredObject(ctx, ws, key); err != nil {
			return err
		}
	}
	return nil
}

// ReconcileRetiredObject removes keypair demand for a retired, complete Task,
// Pass or Execution. It returns true once demand can end. The caller owns the
// write transaction; workers call it on object revisions so final result writes
// and cancellation drain finish before their controllers are released.
func ReconcileRetiredObject(ctx context.Context, ws world.WorldState, objectKey string) (bool, error) {
	// Leave ordinary reactive objects and unfinished retirement requests alone.
	requested, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(objectKey, PredObjectRetired.String(), objectKey, ""), 1)
	if err != nil || len(requested) == 0 {
		return false, err
	}
	objectType, err := world_types.GetObjectType(ctx, ws, objectKey)
	if err != nil {
		return false, err
	}

	// Read the complete state from the current transaction before removing demand.
	var complete bool
	switch objectType {
	case TaskTypeID:
		body, err := LookupTaskBody(ctx, ws, objectKey)
		if err != nil {
			return false, err
		}
		complete = body.IsComplete()
	case forge_pass.PassTypeID:
		body, _, err := forge_pass.LookupPass(ctx, ws, objectKey)
		if err != nil {
			return false, err
		}
		complete = body.IsComplete()
	case forge_execution.ExecutionTypeID:
		body, err := world.LookupObjectBody[*forge_execution.Execution](ctx, ws, objectKey, forge_execution.NewExecutionBlock)
		if err != nil {
			return false, err
		}
		complete = body.IsComplete()
	}
	if !complete {
		return false, nil
	}

	// Remove only assignment edges, preserving result and custody history.
	links, err := ws.LookupGraphQuads(ctx, identity_world.NewObjectToKeypairQuad(objectKey, ""), 0)
	if err != nil {
		return false, err
	}
	for _, link := range links {
		if err := ws.DeleteGraphQuad(ctx, link); err != nil {
			return false, err
		}
	}
	return true, nil
}

// ReactivateTask restores worker demand when an explicit retry reopens a retired
// Task. Historical Passes and Executions remain retired. The caller owns the
// write transaction and has already validated the retry's predecessor.
func ReactivateTask(ctx context.Context, ws world.WorldState, taskKey string, assignedPeer string, sender peer.ID) error {
	// Clear the Task's retirement intent without changing its historical attempts.
	retired := world.NewGraphQuadWithKeys(taskKey, PredObjectRetired.String(), taskKey, "")
	links, err := ws.LookupGraphQuads(ctx, retired, 1)
	if err != nil || len(links) == 0 {
		return err
	}
	if err := ws.DeleteGraphQuad(ctx, retired); err != nil {
		return err
	}

	// Restore the Task's assigned keypair so its worker can admit the next Pass.
	if assignedPeer == "" {
		return nil
	}
	id, err := confparse.ParsePeerID(assignedPeer)
	if err != nil {
		return err
	}
	_, _, err = identity_world.LinkObjectToKeypair(ctx, ws, sender, taskKey, id, "", nil)
	return err
}

// InheritRetirement carries a parent's one-shot retirement intent into an
// attempt created while its final outcome is still draining. The caller owns
// the creation transaction; ordinary reactive parents leave no retirement edge.
func InheritRetirement(ctx context.Context, ws world.WorldState, parentKey, childKey string) error {
	// Read retirement intent from the same transaction that creates the attempt.
	requested, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(parentKey, PredObjectRetired.String(), parentKey, ""), 1)
	if err != nil || len(requested) == 0 {
		return err
	}

	// Keep the new attempt assigned until it writes its own complete result.
	return ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(childKey, PredObjectRetired.String(), childKey, ""))
}
