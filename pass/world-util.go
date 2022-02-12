package forge_pass

import (
	"context"
	"errors"

	forge_execution "github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/cayleygraph/cayley"
	"github.com/sirupsen/logrus"
)

// WaitPassComplete waits until the Pass is in the COMPLETE state.
func WaitPassComplete(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	passObjectKey string,
) (*Pass, error) {
	// wait for Pass to complete
	var finalState *Pass
	var lastState State
	loop := world_control.NewObjectLoop(
		le,
		ws,
		false,
		passObjectKey,
		world_control.NewWaitForStateHandler(
			func(obj world.ObjectState, rootCs *block.Cursor, rev uint64) (bool, error) {
				if obj == nil {
					return true, nil
				}
				pass, err := UnmarshalPass(rootCs)
				if err != nil {
					return false, err
				}
				nextState := pass.GetPassState()
				if nextState != lastState {
					lastState = nextState
					le.Debugf("pass is in state: %s", nextState.String())
					if ferr := pass.GetResult().GetFailError(); ferr != "" {
						le.WithError(errors.New(ferr)).Warn("pass failed")
					}
				}
				complete := pass.IsComplete()
				if complete {
					finalState = pass
				}
				return !complete, nil
			},
		),
	)
	if err := loop.Execute(ctx); err != nil {
		return nil, err
	}
	return finalState, nil
}

// ListPassExecutions lists all Execution object keys that are linked to by the Pass.
func ListPassExecutions(ctx context.Context, w world.WorldState, passKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		passKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			return p.Out(PredPassToExecution), nil
		},
	)
}

// CollectExecutions collects all Executions linked to by the Pass.
// If any of the linked states are invalid, returns an error.
func CollectPassExecutions(
	ctx context.Context,
	ws world.WorldState,
	passObjectKeys ...string,
) ([]*forge_execution.Execution, []string, error) {
	kpObjectKeys, err := ListPassExecutions(ctx, ws, passObjectKeys...)
	if err != nil {
		return nil, nil, err
	}

	states := make([]*forge_execution.Execution, len(kpObjectKeys))
	for i, objKey := range kpObjectKeys {
		states[i], err = forge_execution.LookupExecution(ctx, ws, objKey)
		if err != nil {
			return nil, nil, err
		}
	}

	return states, kpObjectKeys, nil
}
