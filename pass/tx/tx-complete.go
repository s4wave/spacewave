package pass_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_pass "github.com/aperturerobotics/forge/pass"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// NewTxComplete constructs the COMPLETE transaction.
func NewTxComplete(objKey string, result *forge_value.Result) *Tx {
	return &Tx{
		PassObjectKey: objKey,

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

// ExecuteTx executes the transaction against the pass instance.
func (t *TxComplete) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	sender peer.ID,
	objKey string,
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

		// promote the first successful exec state value set to the pass
		execStates := root.GetExecStates()
		if len(execStates) == 0 {
			return errors.New("exec_states cannot be empty")
		}

		var successfulState *forge_pass.ExecState
		for _, st := range execStates {
			if st.GetResult().IsSuccessful() {
				successfulState = st
			}
		}
		if successfulState == nil {
			return errors.New("exec_states must contain at least one successful state")
		}

		if root.ValueSet == nil {
			root.ValueSet = &forge_target.ValueSet{}
		}

		// TODO TODO TODO TODO Rather than copy the execution state outputs, we
		// need to use the mappings of outputs from the Target object.
		stValueSet := successfulState.GetValueSet().Clone()
		root.ValueSet.Outputs = stValueSet.GetOutputs()
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
