package execution_transaction

import (
	"context"

	"github.com/aperturerobotics/bifrost/util/confparse"
	forge_execution "github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/hydra/block"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
)

// NewTxStart constructs a new START transaction.
func NewTxStart(peerID peer.ID) *TxStart {
	return &TxStart{
		PeerId: peerID.Pretty(),
	}
}

// NewTxStartTxn constructs a new START transaction.
func NewTxStartTxn() Transaction {
	return &TxStart{}
}

// GetExecutionTransactionType returns the type of transaction this is.
func (t *TxStart) GetExecutionTransactionType() ExecutionTxType {
	return ExecutionTxType_EXECUTION_TX_TYPE_START
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
	exCursor *block.Cursor,
	root *forge_execution.Execution,
) error {
	// ensure PENDING
	if root.GetExecutionState() != forge_execution.State_ExecutionState_PENDING {
		return errors.Errorf(
			"cannot start execution in state: %s",
			root.GetExecutionState().String(),
		)
	}

	// promote to RUNNING
	root.ExecutionState = forge_execution.State_ExecutionState_RUNNING
	exCursor.SetBlock(root, true)

	return nil
}

// ParsePeerID parses the peer ID field.
func (t *TxStart) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(t.GetPeerId())
}

func init() {
	addTransConst(ExecutionTxType_EXECUTION_TX_TYPE_START, NewTxStartTxn)
}

// _ is a type assertion
var (
	_ Transaction = ((*TxStart)(nil))
)
