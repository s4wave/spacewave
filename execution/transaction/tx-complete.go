package execution_transaction

import (
	"context"
	"errors"

	forge_execution "github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/hydra/block"
)

// NewTxComplete constructs the COMPLETE transaction.
func NewTxComplete(result *forge_execution.Result) *TxComplete {
	return &TxComplete{
		Result: result,
	}
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
// txCursor should be located at the transaction.
// exCursor should be located at the execution state root.
// The transaction may be traversed via txCursor.
// The result is written into exCursor.
// The results will be saved if !dryRun.
// If sysErr == true, tx is not marked invalid and will retry.
func (t *TxComplete) ExecuteTx(
	ctx context.Context,
	txCursor *block.Cursor,
	exCursor *block.Cursor,
	root *forge_execution.Execution,
	dryRun bool,
) (sysErr bool, err error) {
	err = errors.New("TODO TxComplete ExecuteTX")
	return
}

// _ is a type assertion
var (
	_ Transaction = ((*TxComplete)(nil))
)
