package execution_transaction

import (
	"context"
	"errors"

	forge_execution "github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
)

// NewTxSetOutputsTxn constructs a new SET_OUTPUTS transaction.
func NewTxSetOutputsTxn() Transaction {
	return &TxSetOutputs{}
}

// GetExecutionTransactionType returns the type of transaction this is.
func (t *TxSetOutputs) GetExecutionTransactionType() ExecutionTxType {
	return ExecutionTxType_EXECUTION_TX_TYPE_SET_OUTPUTS
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxSetOutputs) Validate() error {
	outputs := forge_value.ValueSlice(t.GetOutputs())
	if err := outputs.Validate(true); err != nil {
		return err
	}
	return nil
}

// ExecuteTx executes the transaction against the execution instance.
func (t *TxSetOutputs) ExecuteTx(
	ctx context.Context,
	exCursor *block.Cursor,
	root *forge_execution.Execution,
) error {
	return errors.New("TODO TxSetOutputs ExecuteTX")
}

func init() {
	addTransConst(ExecutionTxType_EXECUTION_TX_TYPE_SET_OUTPUTS, NewTxSetOutputsTxn)
}

// _ is a type assertion
var (
	_ Transaction = ((*TxSetOutputs)(nil))
)
