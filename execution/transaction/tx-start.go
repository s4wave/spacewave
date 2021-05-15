package execution_transaction

import (
	"context"
	"errors"

	"github.com/aperturerobotics/bifrost/util/confparse"
	forge_execution "github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/hydra/block"
	"github.com/libp2p/go-libp2p-core/peer"
)

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
// txCursor should be located at the transaction.
// exCursor should be located at the execution state root.
// The transaction may be traversed via txCursor.
// The result is written into exCursor.
// The results will be saved if !dryRun.
// If sysErr == true, tx is not marked invalid and will retry.
func (t *TxStart) ExecuteTx(
	ctx context.Context,
	txCursor *block.Cursor,
	exCursor *block.Cursor,
	root *forge_execution.Execution,
	dryRun bool,
) (sysErr bool, err error) {
	err = errors.New("TODO TxStart ExecuteTX")
	return
}

// ParsePeerID parses the peer ID field.
func (t *TxStart) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(t.GetPeerId())
}

// _ is a type assertion
var (
	_ Transaction = ((*TxStart)(nil))
)
