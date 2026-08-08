package execution_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxCancel constructs a cancellation transaction.
func NewTxCancel() *Tx {
	return &Tx{
		TxType:   TxType_TxType_CANCEL,
		TxCancel: &TxCancel{},
	}
}

// NewTxCancelTxn constructs a cancellation transaction payload.
func NewTxCancelTxn() Transaction {
	return &TxCancel{}
}

// GetTxType returns the type of transaction this is.
func (t *TxCancel) GetTxType() TxType {
	return TxType_TxType_CANCEL
}

// Validate performs a cursory check of the transaction.
func (t *TxCancel) Validate() error {
	return nil
}

// ExecuteTx records a durable cancellation request.
func (t *TxCancel) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	exCursor *block.Cursor,
	root *forge_execution.Execution,
) error {
	switch state := root.GetExecutionState(); state {
	case forge_execution.State_ExecutionState_PENDING:
		root.ExecutionState = forge_execution.State_ExecutionState_COMPLETE
		root.Result = forge_value.NewResultWithCanceled(errors.New("execution canceled before start"))
	case forge_execution.State_ExecutionState_RUNNING:
		root.ExecutionState = forge_execution.State_ExecutionState_CANCELING
	case forge_execution.State_ExecutionState_CANCELING,
		forge_execution.State_ExecutionState_COMPLETE:
		return nil
	default:
		return errors.Wrapf(forge_value.ErrUnknownState, "%s", state.String())
	}

	exCursor.SetBlock(root, true)
	return root.Validate()
}

// _ is a type assertion
var _ Transaction = (*TxCancel)(nil)
