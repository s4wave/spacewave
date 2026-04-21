package execution_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxAppendLog constructs an APPEND_LOG transaction.
// Clones the entries when building the Tx object.
func NewTxAppendLog(entries []*forge_execution.LogEntry) (*Tx, error) {
	if len(entries) == 0 {
		return nil, errors.New("entries cannot be empty")
	}
	cloned := make([]*forge_execution.LogEntry, len(entries))
	for i, e := range entries {
		if e == nil {
			return nil, errors.Errorf("entries[%d]: cannot be nil", i)
		}
		cloned[i] = e.CloneVT()
	}
	return &Tx{
		TxType: TxType_TxType_APPEND_LOG,
		TxAppendLog: &TxAppendLog{
			Entries: cloned,
		},
	}, nil
}

// GetTxType returns the type of transaction this is.
func (t *TxAppendLog) GetTxType() TxType {
	return TxType_TxType_APPEND_LOG
}

// Validate performs a cursory check of the transaction.
func (t *TxAppendLog) Validate() error {
	if len(t.GetEntries()) == 0 {
		return errors.New("entries cannot be empty")
	}
	return nil
}

// ExecuteTx executes the transaction against the execution instance.
func (t *TxAppendLog) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	exCursor *block.Cursor,
	root *forge_execution.Execution,
) error {
	// check peer id if set
	if len(sender) != 0 {
		if err := root.CheckPeerID(sender); err != nil {
			return err
		}
	}

	// ensure RUNNING state
	if state := root.GetExecutionState(); state != forge_execution.State_ExecutionState_RUNNING {
		return errors.Wrapf(
			forge_value.ErrUnknownState,
			"%s", state.String(),
		)
	}

	root.LogEntries = append(root.LogEntries, t.GetEntries()...)
	exCursor.SetBlock(root, true)
	return nil
}

// _ is a type assertion
var _ Transaction = ((*TxAppendLog)(nil))
