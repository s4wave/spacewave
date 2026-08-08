package pass_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
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
	executions, _, err := forge_pass.CollectPassExecutions(ctx, worldState, objKey)
	if err != nil {
		return err
	}
	for _, execution := range executions {
		if !execution.IsComplete() {
			return errors.Errorf(
				"cannot complete pass while execution is %s",
				execution.GetExecutionState().String(),
			)
		}
	}

	result := t.GetResult()
	if result == nil {
		result = &forge_value.Result{}
	}
	result.FillFailError()

	passState := root.GetPassState()
	if passState == forge_pass.State_PassState_COMPLETE {
		return nil
	}
	if result.GetCanceled() {
		if passState != forge_pass.State_PassState_CANCELING {
			return errors.Errorf(
				"%s: canceled completion requires CANCELING state",
				passState.String(),
			)
		}
	} else if passState != forge_pass.State_PassState_CHECKING {
		return errors.Errorf(
			"%s: non-canceled completion requires CHECKING state",
			passState.String(),
		)
	}

	if result.IsSuccessful() {
		tgt, _, err := root.FollowTargetRef(ctx, bcs)
		if err != nil {
			return err
		}
		outputs := tgt.GetOutputs()
		outpVals, err := forge_pass.ComputeOutputsWithStates(outputs, root.GetExecStates(), int(root.GetReplicas()))
		if err != nil {
			return err
		}
		if root.ValueSet == nil {
			root.ValueSet = &forge_target.ValueSet{}
		}
		root.ValueSet.Outputs = outpVals
	}

	root.PassState = forge_pass.State_PassState_COMPLETE
	root.Result = result
	bcs.SetBlock(root, true)

	return nil
}

// _ is a type assertion
var _ Transaction = (*TxComplete)(nil)
