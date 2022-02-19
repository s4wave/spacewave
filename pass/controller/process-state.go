package pass_controller

import (
	"context"

	forge_pass "github.com/aperturerobotics/forge/pass"
	pass_transaction "github.com/aperturerobotics/forge/pass/tx"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ProcessState implements the state reconciliation loop.
func (c *Controller) ProcessState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	objKey := c.objKey
	if obj == nil {
		le.Debug("object does not exist, waiting")
		return true, nil
	}

	// unmarshal Pass state + build read cursor
	var passState *forge_pass.Pass
	var tgt *forge_target.Target
	_, err = world.AccessObject(ctx, ws.AccessWorldState, rootRef, func(bcs *block.Cursor) error {
		var berr error
		passState, berr = forge_pass.UnmarshalPass(bcs)
		if berr != nil {
			return berr
		}

		tgt, _, berr = passState.FollowTargetRef(bcs)
		return berr
	})
	if err != nil {
		return false, err
	}
	_ = tgt

	// signal to the controller to stop watching for exec states
	currState := passState.GetPassState()
	if currState != forge_pass.State_PassState_RUNNING {
		c.pushWatchExecStates(nil)
	}

	// check if completed
	if currState == forge_pass.State_PassState_COMPLETE {
		le.Debug("pass is marked as complete")
		return false, nil
	}

	// check if peer id matches
	if c.peerIDStr != passState.GetPeerId() {
		le.Warnf("pass peer id %q does not match ours %q", passState.GetPeerId(), c.peerIDStr)
		return true, nil
	}

	execStates := passState.GetExecStates()
	if currState == forge_pass.State_PassState_CHECKING {
		le.Debug("TODO check pass execution outputs")

		// asserts that len(execStates) != 0
		if err := passState.Validate(false); err != nil {
			// COMPLETE w/ success=false
			txd := pass_transaction.NewTxComplete(objKey, forge_value.NewResultWithError(err))
			_, _, err = ws.ApplyWorldOp(txd, c.peerID)
			return false, err
		}

		// verify that the outputs look correct
		// currently: we check that the output hashes match.
		exState := execStates[0]

		// build the output set according to the target
		// TODO TODO
		_ = exState

		// COMPLETE w/ success=true
		// this will use the values from the first ExecState
		txd := pass_transaction.NewTxComplete(objKey, forge_value.NewResultWithSuccess())
		_, _, err = ws.ApplyWorldOp(txd, c.peerID)
		return true, err
	}

	// promote pending -> running
	if currState == forge_pass.State_PassState_PENDING {
		var execSpecs []*pass_transaction.ExecSpec
		if len(execStates)+len(execSpecs) < int(passState.GetReplicas()) {
			if c.conf.GetAssignSelf() {
				execSpecs = []*pass_transaction.ExecSpec{{
					PeerId: c.peerID.Pretty(),
				}}
			}
		}

		// apply the transaction to start the executions
		// the control loop will see the change & run ProcessState again
		le.Debug("starting pass")
		txd := pass_transaction.NewTxStart(objKey, execSpecs, true)
		_, _, err = ws.ApplyWorldOp(txd, c.peerID)
		return true, err
	}

	if currState == forge_pass.State_PassState_RUNNING {
		le.Debug("waiting for pass executions to complete")

		// signal to the controller to start / update watchers
		c.pushWatchExecStates(passState.GetExecStates())
		return true, nil
	}

	// unknown state
	return true, errors.Wrapf(
		forge_value.ErrUnknownState,
		"%s", currState.String(),
	)
}

// _ is a type assertion
var _ world_control.ObjectLoopHandler = ((*Controller)(nil)).ProcessState
