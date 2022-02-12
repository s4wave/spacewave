package forge_execution

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/sirupsen/logrus"
)

// LookupExecution looks up an execution in the world.
func LookupExecution(ctx context.Context, ws world.WorldState, objKey string) (*Execution, error) {
	obj, err := world.MustGetObject(ws, objKey)
	if err != nil {
		return nil, err
	}
	var exec *Execution
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		exec, err = UnmarshalExecution(bcs)
		return err
	})
	return exec, err
}

// WaitExecutionComplete waits until the execution is in the COMPLETE state.
func WaitExecutionComplete(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	executionObjectKey string,
) (*Execution, error) {
	// wait for execution to complete
	var finalState *Execution
	var lastState State
	loop := world_control.NewObjectLoop(
		le,
		ws,
		false,
		executionObjectKey,
		world_control.NewWaitForStateHandler(
			func(obj world.ObjectState, rootCs *block.Cursor, rev uint64) (bool, error) {
				if obj == nil {
					return true, nil
				}
				exec, err := UnmarshalExecution(rootCs)
				if err != nil {
					return false, err
				}
				nextState := exec.GetExecutionState()
				if nextState != lastState {
					lastState = nextState
					le.Debugf("execution is in state: %s", nextState.String())
					if ferr := exec.GetResult().GetFailError(); ferr != "" {
						le.WithError(errors.New(ferr)).Warn("execution failed")
					}
				}
				complete := exec.IsComplete()
				if complete {
					// finalState, _ = proto.Clone(exec).(*Execution)
					finalState = exec
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
