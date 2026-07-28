package execution_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxComplete constructs the COMPLETE transaction.
func NewTxComplete(result *forge_value.Result, claims ...*forge_execution.Claim) *Tx {
	claim := claimOrImplicit(claims)
	return &Tx{
		TxType: TxType_TxType_COMPLETE,
		TxComplete: &TxComplete{
			Result:     result,
			ClaimId:    claim.GetClaimId(),
			ClaimEpoch: claim.GetEpoch(),
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
	if t.GetClaimId() == "" {
		return errors.New("claim_id cannot be empty")
	}
	if t.GetClaimEpoch() == 0 {
		return &StaleClaimEpochError{}
	}
	return nil
}

// ExecuteTx executes the transaction against the execution instance.
func (t *TxComplete) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	exCursor *block.Cursor,
	root *forge_execution.Execution,
) error {
	if len(sender) != 0 {
		if err := root.CheckPeerID(sender); err != nil {
			return err
		}
	}
	if err := checkClaim(root.GetClaim(), t.GetClaimId(), t.GetClaimEpoch()); err != nil {
		return err
	}
	// The same claim may finish after its result has already been persisted.
	// Preserve that authoritative result.
	execState := root.GetExecutionState()
	if execState == forge_execution.State_ExecutionState_COMPLETE {
		return nil
	}
	result := t.GetResult()
	switch execState {
	case forge_execution.State_ExecutionState_RUNNING:
		if result.GetCanceled() {
			return errors.New("canceled completion requires CANCELING state")
		}
	case forge_execution.State_ExecutionState_CANCELING:
		if !result.GetCanceled() {
			return errors.New("CANCELING execution requires a canceled result")
		}
	default:
		return errors.Errorf(
			"cannot complete execution in state: %s",
			execState.String(),
		)
	}

	if result == nil {
		result = &forge_value.Result{}
	}
	result.FillFailError()

	// promote to COMPLETE
	root.ExecutionState = forge_execution.State_ExecutionState_COMPLETE
	root.Result = result
	exCursor.SetBlock(root, true)

	if err := root.Validate(); err != nil {
		return err
	}

	return nil
}

// _ is a type assertion
var _ Transaction = ((*TxComplete)(nil))
