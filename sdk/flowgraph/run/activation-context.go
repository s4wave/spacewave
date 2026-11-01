package flowgraph_run

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// contextInput builds the Task-owned context value and its Target alias.
func (r *runner) contextInput(ctx context.Context, target *forge_target.Target, taskKey, key, nodeID string) (*forge_value.Value, error) {
	// Capture the template from the same revision as the run's authored graph.
	placement := r.run.GetPlacements()[nodeID].First()
	activation := &s4wave_flowgraph.FlowgraphActivationContext{RunKey: r.runKey, ActivationId: taskKey, Key: key}
	if placement.GetKind() == s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR_TEMPLATE {
		activation.TemplateKey = placement.GetObjectKey()
	}

	// Build the context in the Task's World bucket. The caller adopts its
	// relative reference in Task.ValueSet, whose inputs retain their blocks.
	// Target inputs describe resolution and do not own value block references.
	ref, err := world.AccessObject(ctx, r.ws.AccessWorldState, nil, func(cursor *block.Cursor) error {
		cursor.SetBlock(activation, true)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Resolve the context from the Task's scheduler-supplied input values.
	target.Inputs = slices.DeleteFunc(target.Inputs, func(input *forge_target.Input) bool {
		return input.GetName() == s4wave_flowgraph.ActivationContextInputName
	})
	target.Inputs = append(target.Inputs, &forge_target.Input{
		Name:      s4wave_flowgraph.ActivationContextInputName,
		InputType: forge_target.InputType_InputType_ALIAS,
	})
	return forge_value.NewValueWithBlockRef(s4wave_flowgraph.ActivationContextInputName, ref.GetRootRef()), nil
}

// ReadActivationContext reads the reserved Task input during exec initialization.
// The context remains stable across execution attempts of the same activation.
func ReadActivationContext(ctx context.Context, inputs forge_target.InputMap, handle forge_target.ExecControllerHandle) (*s4wave_flowgraph.FlowgraphActivationContext, error) {
	// Require the context supplied by the owning Flowgraph run.
	value, err := forge_target.InputValueToValue(inputs[s4wave_flowgraph.ActivationContextInputName])
	if err != nil {
		return nil, err
	}
	if value == nil || value.IsEmpty() {
		return nil, errors.New("Flowgraph activation context is missing")
	}

	// Decode the typed block through the exec's storage handle.
	var activation *s4wave_flowgraph.FlowgraphActivationContext
	_, err = forge_target.AccessValue(ctx, handle, value, func(cursor *block.Cursor) error {
		var err error
		activation, err = block.UnmarshalBlock[s4wave_flowgraph.FlowgraphActivationContext](ctx, cursor)
		return err
	})
	return activation, err
}
