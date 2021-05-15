package execution_transaction

import (
	"context"
	"errors"

	forge_execution "github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
)

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxSetOutputs) Validate() error {
	outputs := forge_value.ValueSlice(t.GetOutputs())
	if err := outputs.Validate(true); err != nil {
		return err
	}
	return nil
}

// GetExecutionTransactionType returns the type of transaction this is.
func (t *TxSetOutputs) GetExecutionTransactionType() ExecutionTxType {
	return ExecutionTxType_EXECUTION_TX_TYPE_START
}

// ExecuteTx executes the transaction against the execution instance.
// txCursor should be located at the transaction.
// exCursor should be located at the execution state root.
// The transaction may be traversed via txCursor.
// The result is written into exCursor.
// The results will be saved if !dryRun.
// If sysErr == true, tx is not marked invalid and will retry.
func (t *TxSetOutputs) ExecuteTx(
	ctx context.Context,
	txCursor *block.Cursor,
	exCursor *block.Cursor,
	root *forge_execution.Execution,
	dryRun bool,
) (sysErr bool, err error) {
	err = errors.New("TODO TxStart ExecuteTX")
	return
}

// _ is a type assertion
var (
	_ Transaction = ((*TxSetOutputs)(nil))
)
