package execution_transaction

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_execution "github.com/aperturerobotics/forge/execution"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
)

// NewTxComplete constructs the COMPLETE transaction.
func NewTxComplete(result *forge_value.Result) *TxComplete {
	return &TxComplete{
		Result: result,
	}
}

// NewTxCompleteTxn constructs the COMPLETE transaction.
func NewTxCompleteTxn() Transaction {
	return &TxComplete{}
}

// GetExecutionTransactionType returns the type of transaction this is.
func (t *TxComplete) GetExecutionTransactionType() ExecutionTxType {
	return ExecutionTxType_EXECUTION_TX_TYPE_COMPLETE
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxComplete) Validate() error {
	if err := t.GetResult().Validate(); err != nil {
		return err
	}
	return nil
}

// ExecuteTx executes the transaction against the execution instance.
func (t *TxComplete) ExecuteTx(
	ctx context.Context,
	executorPeerID peer.ID,
	exCursor *block.Cursor,
	root *forge_execution.Execution,
) error {
	// ensure RUNNING
	if root.GetExecutionState() != forge_execution.State_ExecutionState_RUNNING {
		return errors.Errorf(
			"cannot complete execution in state: %s",
			root.GetExecutionState().String(),
		)
	}

	result := t.GetResult()
	if result == nil {
		result = &forge_value.Result{}
	}
	if !result.GetSuccess() && len(result.GetFailError()) == 0 {
		result.FailError = errors.New("execution failed without error details").Error()
	}

	// promote to COMPLETE
	root.ExecutionState = forge_execution.State_ExecutionState_COMPLETE
	root.Result = result
	exCursor.SetBlock(root, true)

	return nil
}

func init() {
	addTransConst(ExecutionTxType_EXECUTION_TX_TYPE_COMPLETE, NewTxCompleteTxn)
}

// _ is a type assertion
var (
	_ Transaction = ((*TxComplete)(nil))
)
