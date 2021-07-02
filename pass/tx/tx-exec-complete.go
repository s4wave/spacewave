package pass_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_pass "github.com/aperturerobotics/forge/pass"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// NewTxExecComplete constructs a new EXEC_COMPLETE transaction.
func NewTxExecComplete() *Tx {
	return &Tx{
		TxType:         TxType_TxType_EXEC_COMPLETE,
		TxExecComplete: &TxExecComplete{},
	}
}

// NewTxExecCompleteTxn constructs a new EXEC_COMPLETE transaction.
func NewTxExecCompleteTxn() Transaction {
	return &TxExecComplete{}
}

// GetTxType returns the type of transaction this is.
func (t *TxExecComplete) GetTxType() TxType {
	return TxType_TxType_EXEC_COMPLETE
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxExecComplete) Validate() error {
	return nil
}

// ExecuteTx executes the transaction against the execution instance.
func (t *TxExecComplete) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	executorPeerID peer.ID,
	bcs *block.Cursor,
	root *forge_pass.Pass,
) error {
	// ensure RUNNING state
	passState := root.GetPassState()
	if passState != forge_pass.State_PassState_RUNNING {
		return errors.Wrapf(
			forge_value.ErrUnknownState,
			"%s", passState.String(),
		)
	}

	return errors.New("TODO pass tx exec complete")
}

// _ is a type assertion
var _ Transaction = ((*TxExecComplete)(nil))
