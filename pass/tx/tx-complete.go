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

// NewTxComplete constructs the COMPLETE transaction.
func NewTxComplete(result *forge_value.Result) *Tx {
	return &Tx{
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

// ExecuteTx executes the transaction against the execution instance.
func (t *TxComplete) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	executorPeerID peer.ID,
	bcs *block.Cursor,
	root *forge_pass.Pass,
) error {
	// ensure CHECKING state if the result is not failed
	passState := root.GetPassState()
	isSuccess := t.GetResult().IsSuccessful()
	if isSuccess {
		if passState != forge_pass.State_PassState_CHECKING {
			return errors.Errorf(
				"%s: must be in CHECKING state if completing successfully",
				passState.String(),
			)
		}
	} else {
		if passState == forge_pass.State_PassState_COMPLETE {
			return errors.Wrapf(
				forge_value.ErrUnknownState,
				"%s", passState.String(),
			)
		}
	}

	result := t.GetResult()
	if result == nil {
		result = &forge_value.Result{}
	}
	result.FillFailError()

	// promote to COMPLETE
	root.PassState = forge_pass.State_PassState_COMPLETE
	root.Result = result
	bcs.SetBlock(root, true)

	return nil
}

// _ is a type assertion
var _ Transaction = ((*TxComplete)(nil))
