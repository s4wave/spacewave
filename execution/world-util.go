package forge_execution

import (
	"context"

	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/hydra/world/control"
	"github.com/gogo/protobuf/proto"
	"github.com/sirupsen/logrus"
)

// WaitExecutionComplete waits until the execution is in the COMPLETE state.
func WaitExecutionComplete(
	ctx context.Context,
	le *logrus.Entry,
	eng world.Engine,
	executionObjectID string,
) (*forge_value.Result, error) {
	// wait for execution to complete
	var res *forge_value.Result
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
					if execRes := exec.GetResult(); execRes != nil {
						res, _ = proto.Clone(execRes).(*forge_value.Result)
					}
				}
				return !complete, nil
			},
		),
	)
	if err := loop.Execute(ctx); err != nil {
		return nil, err
	}
	return res, nil
}
