package execution_tx

import (
	"context"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// NewTxReclaim constructs a RECLAIM transaction.
//
// observedAt is when the sender saw the claim lease expire. The new claim stays
// live until leaseExpiresAt unless its holder renews it.
func NewTxReclaim(
	peerID peer.ID,
	claimID string,
	expectedClaimEpoch uint64,
	observedAt, leaseExpiresAt time.Time,
) *Tx {
	return &Tx{
		TxType: TxType_TxType_RECLAIM,
		TxReclaim: &TxReclaim{
			PeerId:             peerID.String(),
			ClaimId:            claimID,
			ExpectedClaimEpoch: expectedClaimEpoch,
			ObservedAt:         timestamp.New(observedAt),
			LeaseExpiresAt:     timestamp.New(leaseExpiresAt),
		},
	}
}

// NewTxReclaimTxn constructs a new RECLAIM transaction.
func NewTxReclaimTxn() Transaction {
	return &TxReclaim{}
}

// GetTxType returns the type of transaction this is.
func (t *TxReclaim) GetTxType() TxType {
	return TxType_TxType_RECLAIM
}

// Validate performs a cursory check of the transaction.
func (t *TxReclaim) Validate() error {
	// Require a peer, claim identity, and expected epoch for reclaim.
	if len(t.GetPeerId()) == 0 {
		return peer.ErrEmptyPeerID
	}
	if _, err := t.ParsePeerID(); err != nil {
		return err
	}
	if t.GetClaimId() == "" {
		return errors.New("claim_id cannot be empty")
	}
	if t.GetExpectedClaimEpoch() == 0 {
		return errors.New("expected claim epoch cannot be zero")
	}
	if t.GetObservedAt() == nil {
		return errors.New("observed_at cannot be empty")
	}
	if !t.GetLeaseExpiresAt().AsTime().After(t.GetObservedAt().AsTime()) {
		return errors.New("lease_expires_at must be after observed_at")
	}
	return nil
}

// ExecuteTx executes the transaction against the execution instance.
func (t *TxReclaim) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	exCursor *block.Cursor,
	root *forge_execution.Execution,
) error {
	// Require the reclaim sender, execution state, and current claim epoch.
	txPeerID, err := t.ParsePeerID()
	if err != nil {
		return err
	}
	if len(txPeerID) == 0 {
		return peer.ErrEmptyPeerID
	}
	if len(sender) != 0 && sender != txPeerID {
		return errors.Errorf(
			"tx body peer id %s must match sender %s",
			txPeerID.String(), sender.String(),
		)
	}
	if err := root.CheckPeerID(txPeerID); err != nil {
		return err
	}
	if err := root.GetExecutionState().EnsureMatches(
		forge_execution.State_ExecutionState_RUNNING,
		forge_execution.State_ExecutionState_CANCELING,
	); err != nil {
		return err
	}
	if err := checkClaimEpoch(root.GetClaim().GetEpoch(), t.GetExpectedClaimEpoch()); err != nil {
		return err
	}
	if root.GetClaim().GetClaimId() == t.GetClaimId() {
		return errors.New("reclaim requires a new claim_id")
	}

	// Require the sender to have observed the current lease expire.
	if !root.GetClaim().LeaseExpired(t.GetObservedAt().AsTime()) {
		return &ClaimLiveError{
			ClaimID:        root.GetClaim().GetClaimId(),
			Epoch:          root.GetClaim().GetEpoch(),
			LeaseExpiresAt: root.GetClaim().GetLeaseExpiresAt().AsTime(),
		}
	}

	// Replace the execution claim and clear its waiting plugin.
	claimEpoch := root.GetClaim().GetEpoch() + 1
	if claimEpoch == 0 {
		return errors.New("execution claim epoch overflow")
	}
	root.Claim = &forge_execution.Claim{
		ClaimId:        t.GetClaimId(),
		Epoch:          claimEpoch,
		LeaseExpiresAt: t.GetLeaseExpiresAt(),
	}
	root.WaitingPluginId = ""
	exCursor.SetBlock(root, true)
	return root.Validate()
}

// ParsePeerID parses the peer ID field.
func (t *TxReclaim) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(t.GetPeerId())
}

// _ is a type assertion
var _ Transaction = (*TxReclaim)(nil)
