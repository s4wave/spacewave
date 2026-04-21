package pass_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxStart constructs a new START transaction.
func NewTxStart(objKey string, execSpecs []*ExecSpec, clearExisting bool) *Tx {
	return &Tx{
		PassObjectKey: objKey,

		TxType: TxType_TxType_START,
		TxStart: &TxStart{
			CreateExecSpecs: &TxCreateExecSpecs{
				ExecSpecs:     execSpecs,
				ClearExisting: clearExisting,
			},
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
	if len(t.GetCreateExecSpecs().GetExecSpecs()) != 0 {
		if err := t.GetCreateExecSpecs().Validate(); err != nil {
			return errors.Wrap(err, "create_exec_specs")
		}
	}
	return nil
}

// ExecuteTx executes the transaction against the pass instance.
func (t *TxStart) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	sender peer.ID,
	objKey string,
	bcs *block.Cursor,
	root *forge_pass.Pass,
) error {
	// ensure PENDING
	passState := root.GetPassState()
	if passState != forge_pass.State_PassState_PENDING {
		return errors.Wrapf(
			forge_value.ErrUnknownState,
			"%s", passState.String(),
		)
	}

	// promote to RUNNING
	root.PassState = forge_pass.State_PassState_RUNNING

	// apply the create specs op if set, otherwise call update exec states
	var err error
	if createSpecs := t.GetCreateExecSpecs(); !createSpecs.IsEmpty() {
		err = createSpecs.ExecuteTx(ctx, worldState, sender, objKey, bcs, root)
	} else {
		updateSpecs := NewTxUpdateExecStatesTxn()
		err = updateSpecs.ExecuteTx(ctx, worldState, sender, objKey, bcs, root)
	}
	if err != nil {
		return err
	}

	// write changes
	bcs.SetBlock(root, true)
	return nil
}

// _ is a type assertion
var _ Transaction = ((*TxStart)(nil))
