package execution_tx

import (
	"context"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxRenewClaim constructs a claim-fenced lease renewal.
func NewTxRenewClaim(claim *forge_execution.Claim, leaseExpiresAt time.Time) *Tx {
	return &Tx{
		TxType: TxType_TxType_RENEW_CLAIM,
		TxRenewClaim: &TxRenewClaim{
			ClaimId:        claim.GetClaimId(),
			ClaimEpoch:     claim.GetEpoch(),
			LeaseExpiresAt: timestamp.New(leaseExpiresAt),
		},
	}
}

// GetTxType returns the lease renewal transaction type.
func (t *TxRenewClaim) GetTxType() TxType { return TxType_TxType_RENEW_CLAIM }

// Validate requires a claim identity and a lease.
func (t *TxRenewClaim) Validate() error {
	if t.GetClaimId() == "" {
		return errors.New("claim_id cannot be empty")
	}
	if t.GetClaimEpoch() == 0 {
		return &StaleClaimEpochError{}
	}
	if t.GetLeaseExpiresAt() == nil {
		return errors.New("lease_expires_at cannot be empty")
	}
	return nil
}

// ExecuteTx extends the lease while the claimed execution is active.
func (t *TxRenewClaim) ExecuteTx(ctx context.Context, sender peer.ID, exCursor *block.Cursor, root *forge_execution.Execution) error {
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

	// Persist the lease only when it extends the current one.
	if !t.GetLeaseExpiresAt().AsTime().After(root.GetClaim().GetLeaseExpiresAt().AsTime()) {
		return nil
	}
	root.Claim.LeaseExpiresAt = t.GetLeaseExpiresAt()
	exCursor.SetBlock(root, true)
	return nil
}

// _ is a type assertion
var _ Transaction = (*TxRenewClaim)(nil)
