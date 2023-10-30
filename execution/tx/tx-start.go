package execution_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	forge_execution "github.com/aperturerobotics/forge/execution"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
)

// NewTxStart constructs a new START transaction.
func NewTxStart(peerID peer.ID) *Tx {
	return &Tx{
		TxType: TxType_TxType_START,
		TxStart: &TxStart{
			PeerId: peerID.String(),
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
	return nil
}

// ExecuteTx executes the transaction against the execution instance.
func (t *TxStart) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	exCursor *block.Cursor,
	root *forge_execution.Execution,
) error {
	// ensure PENDING
	execState := root.GetExecutionState()
	if execState != forge_execution.State_ExecutionState_PENDING {
		return errors.Wrapf(
			forge_value.ErrUnknownState,
			"%s", execState.String(),
		)
	}

	// ensure peer id matches sender peer id
	txPeerID, err := t.ParsePeerID()
	if err != nil {
		return err
	}
	if len(txPeerID) == 0 {
		return peer.ErrEmptyPeerID
	}
	if len(sender) != 0 {
		if sender != txPeerID {
			return errors.Errorf(
				"tx body peer id %s must match sender %s",
				txPeerID.String(), sender.String(),
			)
		}
	}

	// promote to RUNNING
	root.ExecutionState = forge_execution.State_ExecutionState_RUNNING
	exCursor.SetBlock(root, true)

	if err := root.Validate(); err != nil {
		return err
	}

	return nil
}

// ParsePeerID parses the peer ID field.
func (t *TxStart) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(t.GetPeerId())
}

// _ is a type assertion
var _ Transaction = ((*TxStart)(nil))
