package execution_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// NewTxStart constructs a new START transaction.
func NewTxStart(peerID peer.ID, claimIDs ...string) *Tx {
	claimID := implicitClaim.GetClaimId()
	if len(claimIDs) != 0 {
		claimID = claimIDs[0]
	}
	return &Tx{
		TxType: TxType_TxType_START,
		TxStart: &TxStart{
			PeerId:  peerID.String(),
			ClaimId: claimID,
		},
	}
}

// NewTxStartTxn constructs a new START transaction.
func NewTxStartTxn() Transaction {
	return &TxStart{}
}

// GetTxType returns the type of transaction this is.
func (t *TxStart) GetTxType() TxType {
	return TxType_TxType_START
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxStart) Validate() error {
	if len(t.GetPeerId()) == 0 {
		return peer.ErrEmptyPeerID
	}
	if _, err := t.ParsePeerID(); err != nil {
		return err
	}
	if t.GetClaimId() == "" {
		return errors.New("claim_id cannot be empty")
	}
	return nil
}

// ExecuteTx executes the transaction against the execution instance.
func (t *TxStart) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	exCursor *block.Cursor,
	root *forge_execution.Execution,
) error {
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
	if t.GetClaimId() == "" {
		return errors.New("claim_id cannot be empty")
	}

	execState := root.GetExecutionState()
	switch execState {
	case forge_execution.State_ExecutionState_PENDING:
	case forge_execution.State_ExecutionState_RUNNING,
		forge_execution.State_ExecutionState_CANCELING:
		claim := root.GetClaim()
		if claim != nil {
			if claim.GetClaimId() != t.GetClaimId() {
				return &ClaimHeldError{
					ClaimID: claim.GetClaimId(),
					Epoch:   claim.GetEpoch(),
				}
			}
			return nil
		}
	default:
		return errors.Wrapf(
			forge_value.ErrUnknownState,
			"%s", execState.String(),
		)
	}

	claimEpoch := root.GetClaim().GetEpoch() + 1
	if claimEpoch == 0 {
		return errors.New("execution claim epoch overflow")
	}
	root.Claim = &forge_execution.Claim{
		ClaimId: t.GetClaimId(),
		Epoch:   claimEpoch,
	}
	if execState == forge_execution.State_ExecutionState_PENDING {
		root.ExecutionState = forge_execution.State_ExecutionState_RUNNING
	}
	exCursor.SetBlock(root, true)

	return root.Validate()
}

// ParsePeerID parses the peer ID field.
func (t *TxStart) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(t.GetPeerId())
}

// _ is a type assertion
var _ Transaction = ((*TxStart)(nil))
