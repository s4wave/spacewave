package task_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_pass "github.com/aperturerobotics/forge/pass"
	forge_task "github.com/aperturerobotics/forge/task"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
)

// NewTxUpdatePassState constructs a new UPDATE_PASS_STATE transaction.
func NewTxUpdatePassState(objKey string) *Tx {
	return &Tx{
		TaskObjectKey: objKey,

		TxType:            TxType_TxType_UPDATE_PASS_STATE,
		TxUpdatePassState: &TxUpdatePassState{},
	}
}

// NewTxUpdatePassStateTxn constructs a new UPDATE_PASS_STATE transaction.
func NewTxUpdatePassStateTxn() Transaction {
	return &TxUpdatePassState{}
}

// GetTxType returns the type of transaction this is.
func (t *TxUpdatePassState) GetTxType() TxType {
	return TxType_TxType_UPDATE_PASS_STATE
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxUpdatePassState) Validate() error {
	return nil
}

// ExecuteTx executes the transaction against the Task instance.
func (t *TxUpdatePassState) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	sender peer.ID,
	objKey string,
	bcs *block.Cursor,
	root *forge_task.Task,
) error {
	// ensure RUNNING state
	err := root.GetTaskState().EnsureMatches(forge_task.State_TaskState_RUNNING)
	if err != nil {
		return err
	}

	// lookup the current Pass
	currPass, _, _, err := forge_task.LookupTaskPass(ctx, worldState, objKey, root.GetPassNonce())
	if err != nil {
		return err
	}

	// If complete: transitions to CHECKING state.
	currPassState := currPass.GetPassState()
	switch currPassState {
	case forge_pass.State_PassState_UNKNOWN:
		root.TaskState = forge_task.State_TaskState_PENDING
	case forge_pass.State_PassState_COMPLETE:
		root.TaskState = forge_task.State_TaskState_CHECKING
	default:
		return nil
	}

	// mark as dirty
	bcs.SetBlock(root, true)
	return nil
}

// _ is a type assertion
var _ Transaction = ((*TxUpdatePassState)(nil))
