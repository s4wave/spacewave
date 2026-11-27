package execution_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxSetWaiting constructs a claim-fenced update of what the Execution waits
// for. A nil waiting clears the wait.
func NewTxSetWaiting(waiting *forge_target.Waiting, claim *forge_execution.Claim) *Tx {
	return &Tx{
		TxType: TxType_TxType_SET_WAITING,
		TxSetWaiting: &TxSetWaiting{
			Waiting:    waiting,
			ClaimId:    claim.GetClaimId(),
			ClaimEpoch: claim.GetEpoch(),
		},
	}
}

// GetTxType returns the wait update transaction type.
func (t *TxSetWaiting) GetTxType() TxType { return TxType_TxType_SET_WAITING }

// Validate requires a current execution claim and a well-formed wait.
func (t *TxSetWaiting) Validate() error {
	if t.GetClaimId() == "" {
		return errors.New("claim_id cannot be empty")
	}
	if t.GetClaimEpoch() == 0 {
		return &StaleClaimEpochError{}
	}
	return errors.Wrap(t.GetWaiting().Validate(), "waiting")
}

// ExecuteTx writes what the claimed execution waits for while it is active.
func (t *TxSetWaiting) ExecuteTx(ctx context.Context, sender peer.ID, exCursor *block.Cursor, root *forge_execution.Execution) error {
	// Require the sender and claim to match an active execution.
	if len(sender) != 0 {
		if err := root.CheckPeerID(sender); err != nil {
			return err
		}
	}
	if err := checkClaim(root.GetClaim(), t.GetClaimId(), t.GetClaimEpoch()); err != nil {
		return err
	}
	if err := root.GetExecutionState().EnsureMatches(forge_execution.State_ExecutionState_RUNNING, forge_execution.State_ExecutionState_CANCELING); err != nil {
		return err
	}

	// Persist the wait, storing an empty reason as no wait.
	root.Waiting = t.GetWaiting()
	if root.Waiting.GetReason() == nil {
		root.Waiting = nil
	}
	exCursor.SetBlock(root, true)
	return nil
}

// _ is a type assertion
var _ Transaction = (*TxSetWaiting)(nil)
