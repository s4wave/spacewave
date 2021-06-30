package forge_execution

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/gogo/protobuf/proto"
	"github.com/sirupsen/logrus"
)

// WaitExecutionComplete waits until the execution is in the COMPLETE state.
func WaitExecutionComplete(
	ctx context.Context,
	le *logrus.Entry,
	eng world.Engine,
	executionObjectID string,
) (*Execution, error) {
	// wait for execution to complete
	var finalState *Execution
	loop := world_control.NewObjectLoop(
		le,
		eng,
		false,
		executionObjectID,
		world_control.NewWaitForStateHandler(
			func(obj world.ObjectState, rootCs *block.Cursor, rev uint64) (bool, error) {
				if obj == nil {
					return true, nil
				}
				exec, err := UnmarshalExecution(rootCs)
				if err != nil {
					return false, err
				}
				le.WithField("rev", rev).Infof("seen object: %s", exec.String())
				complete := exec.IsComplete()
				if complete {
					finalState, _ = proto.Clone(exec).(*Execution)
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
