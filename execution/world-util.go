package forge_execution

import (
	"context"

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
) (*Result, error) {
	// wait for execution to complete
	var res *Result
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
						res, _ = proto.Clone(execRes).(*Result)
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
