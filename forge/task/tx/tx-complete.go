package task_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
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
	tgt, _, err := root.FollowTargetRef(ctx, bcs)
	if err != nil {
		return err
	}

	result := t.GetResult()
	if result == nil {
		result = &forge_value.Result{}
	}

	var passOutputs forge_value.ValueSlice
	if result.IsSuccessful() {
		// an illegal source state is a precondition violation, not an outcome:
		// reject the operation rather than terminally failing a Task that is
		// still running or that already recorded a successful completion.
		if root.GetTaskState() != forge_task.State_TaskState_CHECKING {
			return errors.Errorf(
				"%s: must be in CHECKING state if completing successfully",
				root.GetTaskState().String(),
			)
		}
		passOutputs, err = validateTaskCompletion(
			ctx,
			worldState,
			objKey,
			root,
			tgt,
		)
		if err != nil {
			result = forge_value.NewResultWithError(err)
		}
	} else if root.GetTaskState() == forge_task.State_TaskState_COMPLETE {
		return errors.Wrapf(
			forge_value.ErrUnknownState,
			"%s", root.GetTaskState().String(),
		)
	}

	result.FillFailError()
	if result.GetSuccess() && len(tgt.GetOutputs()) != 0 {
		if root.ValueSet == nil {
			root.ValueSet = &forge_target.ValueSet{}
		}
		root.ValueSet.Outputs = passOutputs
	}

	root.TaskState = forge_task.State_TaskState_COMPLETE
	root.Result = result
	bcs.SetBlock(root, true)
	return nil
}

func validateTaskCompletion(
	ctx context.Context,
	worldState world.WorldState,
	objKey string,
	root *forge_task.Task,
	tgt *forge_target.Target,
) (forge_value.ValueSlice, error) {
	tpass, _, _, err := forge_task.LookupTaskPass(
		ctx,
		worldState,
		objKey,
		root.GetPassNonce(),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "lookup pass[%d]", root.GetPassNonce())
	}
	if tpass == nil {
		return nil, errors.Wrap(world.ErrObjectNotFound, "task pass")
	}
	if err := tpass.Validate(false); err != nil {
		return nil, errors.Wrap(err, "pass")
	}

	passResult := tpass.GetResult()
	if !passResult.GetSuccess() {
		passResult.FillFailError()
		return nil, errors.Wrap(errors.New(passResult.GetFailError()), "pass failed")
	}
	if tpass.GetPassState() != forge_pass.State_PassState_COMPLETE {
		return nil, errors.Errorf(
			"expected pass[%d] to be complete: %s",
			root.GetPassNonce(),
			tpass.GetPassState().String(),
		)
	}

	outputs := tgt.GetOutputs()
	if len(outputs) == 0 {
		return nil, nil
	}
	passOutputs, err := forge_pass.ComputeOutputsWithStates(
		outputs,
		tpass.GetExecStates(),
		int(root.GetReplicas()),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "pass[%d]: compute outputs", root.GetPassNonce())
	}
	if !passOutputs.Equals(tpass.GetValueSet().GetOutputs()) {
		return nil, errors.Errorf(
			"pass[%d]: outputs mismatch re-computed pass values",
			root.GetPassNonce(),
		)
	}
	if !passOutputs.Equals(root.GetValueSet().GetOutputs()) {
		return nil, errors.Errorf(
			"pass[%d]: outputs mismatch re-computed task values",
			root.GetPassNonce(),
		)
	}
	return passOutputs, nil
}

// _ is a type assertion
var _ Transaction = ((*TxComplete)(nil))
