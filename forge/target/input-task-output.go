package forge_target

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// TaskOutputResolver reads a completed Task's output from the supplied World.
// A missing Task, incomplete Task, or missing output returns nil without error.
// Task supplies this function so Target does not depend on its consuming package.
type TaskOutputResolver func(context.Context, world.WorldState, string, string) (*forge_value.Value, error)

// Validate requires a source Task key and output name.
func (i *InputTaskOutput) Validate() error {
	if i.GetTaskKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if i.GetOutputName() == "" {
		return errors.New("task output name cannot be empty")
	}
	return nil
}

// ResolveValue reads an output through the Task package's resolver.
// Unavailable outputs resolve as empty values until the source completes.
func (i *InputTaskOutput) ResolveValue(ctx context.Context, name string, inputWorld InputValueWorld, resolve TaskOutputResolver) (InputValueInline, error) {
	// Validate the source before reading the Forge World.
	if err := i.Validate(); err != nil {
		return nil, err
	}
	if inputWorld == nil || inputWorld.IsEmpty() {
		return nil, nil
	}
	if resolve == nil {
		return nil, errors.New("task output resolver is required")
	}

	// Copy the completed source output under the reader's input name.
	output, err := resolve(ctx, inputWorld.GetWorldState(), i.GetTaskKey(), i.GetOutputName())
	if err != nil {
		return nil, err
	}
	if output.IsEmpty() {
		return nil, nil
	}
	output = output.CloneVT()
	output.Name = name
	return NewInputValueTaskOutput(output), nil
}
