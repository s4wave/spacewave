package testbed

import (
	"errors"
	"time"

	exec_controller "github.com/aperturerobotics/forge/execution/controller"
	exec_transaction "github.com/aperturerobotics/forge/execution/tx"
	forge_pass "github.com/aperturerobotics/forge/pass"
	pass_controller "github.com/aperturerobotics/forge/pass/controller"
	pass_transaction "github.com/aperturerobotics/forge/pass/tx"
	forge_target "github.com/aperturerobotics/forge/target"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/timestamp"
)

// RunPassWithTarget runs a target using the Pass and Pass controllers.
func (tb *Testbed) RunPassWithTarget(
	tgt *forge_target.Target,
	valueSet *forge_target.ValueSet,
	replicas uint32,
	ts *timestamp.Timestamp,
) (*forge_pass.Pass, error) {
	ctx, le, worldState := tb.Context, tb.Logger, tb.WorldState

	// create the Pass object in the world
	// this can be done in any order (the controller waits for object to be present).
	passObjectKey := "pass/1"
	peerID := tb.Volume.GetPeerID()
	_, _, err := forge_pass.CreatePassWithTarget(
		ctx,
		worldState,
		passObjectKey,
		valueSet,
		tgt,
		replicas,
		ts,
	)
	if err != nil {
		return nil, err
	}

	// construct pass controller & attach to Pass object
	passCtrlCfg := pass_controller.NewConfig(
		tb.EngineID,
		passObjectKey,
		peerID,
	)
	passCtrlCfg.AssignSelf = true
	_, passCtrlRef, err := pass_controller.StartControllerWithConfig(
		ctx,
		tb.Bus,
		passCtrlCfg,
	)
	if err != nil {
		return nil, err
	}
	defer passCtrlRef.Release()

	// add op handlers to bus
	opc := world.NewLookupOpController(
		"pass-tx-ops",
		tb.EngineID,
		pass_transaction.LookupWorldOp,
	)
	go tb.Bus.ExecuteController(ctx, opc)
	// hack: wait for it to start
	<-time.After(time.Millisecond * 100)

	// construct execution controller & attach to the object the pass controller
	// will create. NOTE: this should eventually be replaced with the worker.
	execObjKey := forge_pass.BuildPassExecutionObjKey(passObjectKey, peerID.Pretty())
	execCtrlCfg := exec_controller.NewConfig(
		tb.EngineID,
		execObjKey,
		peerID,
		&forge_target.InputWorld{
			EngineId:        passCtrlCfg.EngineId,
			LookupImmediate: true,
		},
	)
	_, execCtrlRef, err := exec_controller.StartControllerWithConfig(
		ctx,
		tb.Bus,
		execCtrlCfg,
	)
	if err != nil {
		return nil, err
	}
	defer execCtrlRef.Release()

	// add op handlers to bus
	opc = world.NewLookupOpController(
		"exec-tx-ops",
		tb.EngineID,
		exec_transaction.LookupWorldOp,
	)
	go tb.Bus.ExecuteController(ctx, opc)
	// hack: wait for it to start
	<-time.After(time.Millisecond * 100)

	// wait for pass to complete
	finalState, err := forge_pass.WaitPassComplete(
		ctx,
		le.WithField("control-loop", "run-pass-wait-complete"),
		tb.WorldState,
		passObjectKey,
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
