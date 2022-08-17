package task_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_pass "github.com/aperturerobotics/forge/pass"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_task "github.com/aperturerobotics/forge/task"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// NewTxComplete constructs the COMPLETE transaction.
func NewTxComplete(objKey string, result *forge_value.Result) *Tx {
	return &Tx{
		TaskObjectKey: objKey,

		TxType: TxType_TxType_COMPLETE,
		TxComplete: &TxComplete{
			Result: result,
		},
	}
}

// NewTxCompleteTxn constructs the COMPLETE transaction.
func NewTxCompleteTxn() Transaction {
	return &TxComplete{}
}

// GetTxType returns the type of transaction this is.
func (t *TxComplete) GetTxType() TxType {
	return TxType_TxType_COMPLETE
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxComplete) Validate() error {
	if err := t.GetResult().Validate(); err != nil {
		return err
	}
	if t.GetResult().GetSuccess() {
		// check the value is set correctly
		if err := t.GetValueSet().Validate(); err != nil {
			return errors.Wrap(err, "value_set")
		}
	} else {
		// check that the value is empty if not successful
		if len(t.GetValueSet().GetOutputs()) != 0 {
			return errors.New("value_set: outputs must be empty if not successful")
		}
	}
	if len(t.GetValueSet().GetInputs()) != 0 {
		return errors.New("value_set: inputs must be empty")
	}
	return nil
}

// ExecuteTx executes the transaction against the Task instance.
func (t *TxComplete) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	sender peer.ID,
	objKey string,
	bcs *block.Cursor,
	root *forge_task.Task,
) error {
	tgt, _, err := root.FollowTargetRef(bcs)
	if err != nil {
		return err
	}

	// ensure CHECKING state if the result is not failed
	taskState := root.GetTaskState()
	isSuccess := t.GetResult().IsSuccessful()
	if isSuccess {
		if taskState != forge_task.State_TaskState_CHECKING {
			return errors.Errorf(
				"%s: must be in CHECKING state if completing successfully",
				taskState.String(),
			)
		}

		// lookup the successful pass
		tpass, _, _, err := forge_task.LookupTaskPass(ctx, worldState, objKey, root.GetPassNonce())
		if err != nil {
			return errors.Wrapf(err, "lookup pass[%d]", root.GetPassNonce())
		}
		if tpass.GetPassState() != forge_pass.State_PassState_COMPLETE {
			return errors.Errorf(
				"expected pass[%d] to be complete: %s",
				root.GetPassNonce(),
				tpass.GetPassState().String(),
			)
		}

		// compute the outputs from the exec states
		outputs := tgt.GetOutputs()
		passOutputs, err := forge_pass.ComputeOutputsWithStates(outputs, tpass.GetExecStates(), int(root.GetReplicas()))
		if err != nil {
			return errors.Wrapf(err, "pass[%d]: compute outputs", root.GetPassNonce())
		}

		// verify the outputs match what the pass has
		if !passOutputs.Equals(tpass.GetValueSet().GetOutputs()) {
			return errors.Wrapf(err, "pass[%d]: outputs mismatch re-computed values", root.GetPassNonce())
		}
		if root.ValueSet == nil {
			root.ValueSet = &forge_target.ValueSet{}
		}
		root.ValueSet.Outputs = passOutputs
	} else {
		if taskState == forge_task.State_TaskState_COMPLETE {
			return errors.Wrapf(
				forge_value.ErrUnknownState,
				"%s", taskState.String(),
			)
		}
	}

	result := t.GetResult()
	if result == nil {
		result = &forge_value.Result{}
	}
	result.FillFailError()

	// promote to COMPLETE
	root.TaskState = forge_task.State_TaskState_COMPLETE
	root.Result = result
	bcs.SetBlock(root, true)
	return nil
}

// _ is a type assertion
var _ Transaction = ((*TxComplete)(nil))
