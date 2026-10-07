package forge_task

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// ResolveOutput reads a named output of a completed Task.
// Missing Tasks, incomplete Tasks, and missing outputs return nil without error.
func ResolveOutput(ctx context.Context, ws world.WorldState, taskKey, outputName string) (*forge_value.Value, error) {
	// Read the source Task without retaining its World object handle.
	task, err := LookupTaskBody(ctx, ws, taskKey)
	if errors.Is(err, world.ErrObjectNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if task.GetTaskState() != State_TaskState_COMPLETE {
		return nil, nil
	}

	// Return the selected output without copying its referenced blocks.
	for _, output := range task.GetValueSet().GetOutputs() {
		if output.GetName() == outputName {
			return output, nil
		}
	}
	return nil, nil
}
