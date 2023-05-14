package task_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	pass_tx "github.com/aperturerobotics/forge/pass/tx"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_task "github.com/aperturerobotics/forge/task"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// NewTxUpdateInputs constructs a new UPDATE_INPUTS transaction.
func NewTxUpdateInputs(objKey string) *Tx {
	return &Tx{
		TaskObjectKey: objKey,

		TxType:         TxType_TxType_UPDATE_INPUTS,
		TxUpdateInputs: &TxUpdateInputs{},
	}
}

// NewTxUpdateInputsTxn constructs a new UPDATE_INPUTS transaction.
func NewTxUpdateInputsTxn() Transaction {
	return &TxUpdateInputs{}
}

// GetTxType returns the type of transaction this is.
func (t *TxUpdateInputs) GetTxType() TxType {
	return TxType_TxType_UPDATE_INPUTS
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxUpdateInputs) Validate() error {
	valueSet := t.GetValueSet()
	if len(valueSet.GetOutputs()) != 0 {
		return errors.New("value_set: outputs: must be empty")
	}
	if len(valueSet.GetInputs()) == 0 && !t.GetResetInputs() {
		return errors.New("value_set: inputs: must be set if !reset_inputs")
	}
	if err := valueSet.Validate(); err != nil {
		return errors.Wrap(err, "value_set")
	}
	return nil
}

// ExecuteTx executes the transaction against the Task instance.
func (t *TxUpdateInputs) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	sender peer.ID,
	objKey string,
	bcs *block.Cursor,
	root *forge_task.Task,
) error {
	if root == nil {
		return errors.New("unexpected empty root task object")
	}

	// if nothing changed we will exit without writing anything
	var dirty bool

	// check if the target changed
	if t.GetUpdateTarget() {
		// lookup the latest version of the task target
		tgt, _, err := forge_task.LookupTaskTarget(ctx, worldState, objKey)
		if err != nil {
			return errors.Wrap(err, "lookup target")
		}

		// lookup the latest referenced Target block.
		existingTgt, existingTgtBcs, err := root.FollowTargetRef(ctx, bcs)
		if err != nil {
			return errors.Wrap(err, "follow target ref")
		}

		// compare (note: existingTgt and tgt both might be nil)
		if !existingTgt.EqualVT(tgt) {
			dirty = true
			existingTgtBcs.SetBlock(tgt, true)
		}
	}

	// TODO: drop any Input values without a corresponding Input in the Target.

	valueSetBefore := root.GetValueSet()
	if valueSetBefore == nil {
		valueSetBefore = forge_target.NewValueSet()
	}

	var valueSet *forge_target.ValueSet
	if t.GetResetInputs() {
		valueSet = forge_target.NewValueSet()

		// note: if reset_inputs is set, we transition to PENDING unconditionally.
		// dirty = len(valueSetBefore.GetInputs()) != 0
		dirty = true
	} else {
		valueSet = valueSetBefore.Clone()
	}

	for _, setInput := range t.GetValueSet().GetInputs() {
		inputName := setInput.GetName()
		existing, existingIdx := valueSet.LookupInput(inputName)
		if existingIdx != -1 && existing.EqualVT(setInput) {
			continue
		}
		dirty = true

		// value_type == 0 -> this is a delete operation
		if setInput.GetValueType() == 0 {
			if existingIdx != -1 {
				valueSet.Inputs[existingIdx] = valueSet.Inputs[len(valueSet.Inputs)-1]
				valueSet.Inputs[len(valueSet.Inputs)-1] = nil
				valueSet.Inputs = valueSet.Inputs[:len(valueSet.Inputs)-1]
			}
		} else {
			if existingIdx != -1 {
				valueSet.Inputs[existingIdx] = setInput
			} else {
				valueSet.Inputs = append(valueSet.Inputs, setInput)
			}
		}
	}

	// nothing changed
	if !dirty {
		return nil
	}

	// sort by name & mark as dirty
	valueSet.SortValues()
	root.ValueSet = valueSet
	bcs.SetBlock(root, true)

	// check if we need to transition to PENDING
	taskState := root.GetTaskState()
	if taskState == forge_task.State_TaskState_PENDING {
		// no more to do
		return nil
	}

	// if there was a running pass, cancel it.
	if taskState == forge_task.State_TaskState_RUNNING {
		// lookup the current Pass
		currPass, _, passKey, err := forge_task.LookupTaskPass(ctx, worldState, objKey, root.GetPassNonce())
		if err != nil {
			return err
		}

		// if !COMPLETE mark as canceled
		if passKey != "" && !currPass.IsComplete() {
			passCompleteTx := pass_tx.NewTxComplete(
				passKey,
				forge_value.NewResultWithCanceled(errors.New("task inputs changed")),
			)
			_, _, err = worldState.ApplyWorldOp(ctx, passCompleteTx, sender)
			if err != nil {
				return err
			}
		}
	}

	// mark as pending
	root.TaskState = forge_task.State_TaskState_PENDING

	return nil
}

// _ is a type assertion
var _ Transaction = ((*TxUpdateInputs)(nil))
