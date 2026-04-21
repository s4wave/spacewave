package execution_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxSetOutputs constructs a new SET_OUTPUTS transaction.
// clones the ValueSet when building the Tx object.
func NewTxSetOutputs(outputs forge_value.ValueSlice, clearOld bool) (*Tx, error) {
	outSet := make(forge_value.ValueSlice, len(outputs))
	for i, outp := range outputs {
		if outp == nil {
			return nil, errors.Errorf("outputs[%d]: cannot be empty", i)
		}
		if err := outp.Validate(false); err != nil {
			return nil, err
		}
		outSet[i] = outp.Clone()
	}
	outSet.SortByName()
	return &Tx{
		TxType: TxType_TxType_SET_OUTPUTS,
		TxSetOutputs: &TxSetOutputs{
			ClearOld: clearOld,
			Outputs:  outSet,
		},
	}, nil
}

// NewTxSetOutputsTxn constructs a new SET_OUTPUTS transaction.
func NewTxSetOutputsTxn() Transaction {
	return &TxSetOutputs{}
}

// GetTxType returns the type of transaction this is.
func (t *TxSetOutputs) GetTxType() TxType {
	return TxType_TxType_SET_OUTPUTS
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxSetOutputs) Validate() error {
	outputs := forge_value.ValueSlice(t.GetOutputs())
	if err := outputs.Validate(false, true, true); err != nil {
		return err
	}
	return nil
}

// ExecuteTx executes the transaction against the execution instance.
func (t *TxSetOutputs) ExecuteTx(
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

	// TODO: validate to ensure ObjectRefs point to valid locations
	var nextOutputs forge_value.ValueSlice
	outputs := forge_value.ValueSlice(t.GetOutputs()).Clone()
	if t.GetClearOld() {
		nextOutputs = outputs
	} else {
		exOutputs := forge_value.ValueSlice(root.GetValueSet().GetOutputs())
		nextOutputs = exOutputs.Merge(outputs)
	}

	// remove any outputs with type UNKNOWN (0)
	nextOutputs = nextOutputs.RemoveUnknown()

	if root.ValueSet == nil {
		root.ValueSet = &forge_target.ValueSet{}
	}
	root.ValueSet.Outputs = nextOutputs
	root.ValueSet.SortValues()
	exCursor.SetBlock(root, true)

	if err := root.Validate(); err != nil {
		return err
	}

	return nil
}

// _ is a type assertion
var _ Transaction = ((*TxSetOutputs)(nil))
