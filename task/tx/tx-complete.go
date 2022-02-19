package task_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
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
	return nil
}

// ExecuteTx executes the transaction against the Task instance.
func (t *TxComplete) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	executorPeerID peer.ID,
	objKey string,
	bcs *block.Cursor,
	root *forge_task.Task,
) error {
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

		// TODO lookup and promote the successful Pass state to the Task
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

	return errors.New("TODO task tx complete")
}

// _ is a type assertion
var _ Transaction = ((*TxComplete)(nil))
