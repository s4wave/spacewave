package testbed

import (
	"errors"
	"time"

	forge_execution "github.com/aperturerobotics/forge/execution"
	execution_controller "github.com/aperturerobotics/forge/execution/controller"
	execution_transaction "github.com/aperturerobotics/forge/execution/tx"
	forge_target "github.com/aperturerobotics/forge/target"
	"github.com/aperturerobotics/hydra/world"
)

// RunExecutionWithTarget runs a target using the Execution controller only.
func (tb *Testbed) RunExecutionWithTarget(
	tgt *forge_target.Target,
	valueSet *forge_target.ValueSet,
) (*forge_execution.Execution, error) {
	ctx, le, worldState := tb.Context, tb.Logger, tb.WorldState

	// create the Execution object in the world
	// this can be done in any order (the controller waits for object to be present).
	executionObjectKey := "execution/1"
	peerID := tb.Volume.GetPeerID()
	_, _, err := forge_execution.CreateExecutionWithTarget(
		ctx,
		worldState,
		executionObjectKey,
		peerID,
		valueSet,
		tgt,
	)
	if err != nil {
		return nil, err
	}

	// construct execution controller & attach to Execution object
	execCtrlCfg := execution_controller.NewConfig(
		tb.EngineID,
		executionObjectKey,
		peerID,
		tb.EngineID, // use same engine for target
	)
	execCtrlCfg.AllowNonExecController = true
	execCtrl, execCtrlRef, err := execution_controller.StartControllerWithConfig(
		ctx,
		tb.Bus,
		execCtrlCfg,
	)
	if err != nil {
		return nil, err
	}
	defer execCtrlRef.Release()
	_ = execCtrl

	// add op handlers to bus
	opc := world.NewLookupOpController(
		"execution-tx-ops",
		tb.EngineID,
		execution_transaction.LookupWorldOp,
	)
	go tb.Bus.ExecuteController(ctx, opc)
	// hack: wait for it to start
	<-time.After(time.Millisecond * 100)

	// wait for execution to complete
	finalState, err := forge_execution.WaitExecutionComplete(
		ctx,
		le.WithField("control-loop", "run-execution-wait-complete"),
		tb.BusEngine,
		executionObjectKey,
	)
	if err != nil {
		return nil, err
	}

	res := finalState.GetResult()
	if errStr := res.FailError; len(errStr) != 0 {
		return finalState, errors.New(errStr)
	}
	// success
	return finalState, nil
}

// RunMockTargetInTestbed builds a basic mock target and runs it in the testbed.
func RunMockTargetInTestbed() {

}
