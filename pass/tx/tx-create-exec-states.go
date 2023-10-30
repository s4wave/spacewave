package pass_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_execution "github.com/aperturerobotics/forge/execution"
	forge_pass "github.com/aperturerobotics/forge/pass"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_parent "github.com/aperturerobotics/hydra/world/parent"
	"github.com/pkg/errors"
)

// NewTxCreateExecSpecs constructs a new CREATE_EXEC_SPECS transaction.
func NewTxCreateExecSpecs(objKey string) *Tx {
	return &Tx{
		PassObjectKey: objKey,

		TxType:            TxType_TxType_CREATE_EXEC_SPECS,
		TxCreateExecSpecs: &TxCreateExecSpecs{},
	}
}

// NewTxCreateExecSpecsTxn constructs a new CREATE_EXEC_SPECS transaction.
func NewTxCreateExecSpecsTxn() Transaction {
	return &TxUpdateExecStates{}
}

// GetTxType returns the type of transaction this is.
func (t *TxCreateExecSpecs) GetTxType() TxType {
	return TxType_TxType_CREATE_EXEC_SPECS
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxCreateExecSpecs) Validate() error {
	if execSpecs := t.GetExecSpecs(); len(execSpecs) != 0 {
		return ValidateExecSpecs(execSpecs)
	} else if !t.GetClearExisting() {
		return errors.New("exec_specs or clear_existing must be set")
	}
	return nil
}

// IsEmpty checks if the transaction is empty.
func (t *TxCreateExecSpecs) IsEmpty() bool {
	execSpecs := t.GetExecSpecs()
	return len(execSpecs) == 0 && !t.GetClearExisting()
}

// ExecuteTx executes the transaction against the pass instance.
func (t *TxCreateExecSpecs) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	sender peer.ID,
	objKey string,
	bcs *block.Cursor,
	root *forge_pass.Pass,
) error {
	// ensure RUNNING state
	err := root.GetPassState().EnsureMatches(forge_pass.State_PassState_RUNNING)
	if err != nil {
		return err
	}

	if t.IsEmpty() {
		return nil
	}

	// list all existing executions & clear those with the same peer IDs
	// ... but only if they are in a terminal state (not running)
	execObjs, err := forge_pass.ListPassExecutions(ctx, worldState, objKey)
	if err != nil {
		return err
	}

	// if clear is set: delete all execution objects first
	skipExecs := make(map[string]struct{})
	for i, execObjKey := range execObjs {
		// unconditionally clear all existing if set
		if !t.GetClearExisting() {
			// check if the exec object is in a terminal state
			// otherwise, skip deleting it
			var execObj *forge_execution.Execution
			_, _, err := world.AccessWorldObject(ctx, worldState, execObjKey, false, func(bcs *block.Cursor) error {
				var err error
				execObj, err = forge_execution.UnmarshalExecution(ctx, bcs)
				return err
			})
			if err != nil {
				return errors.Wrapf(err, "executions[%d]: %s", i, execObjKey)
			}
			if execObj.GetExecutionState() == forge_execution.State_ExecutionState_RUNNING {
				skipExecs[execObj.GetPeerId()] = struct{}{}
				continue
			}
		}

		if _, err := worldState.DeleteObject(ctx, execObjKey); err != nil {
			return err
		}
	}

	// create / overwrite the execution objects
	parentState := world_parent.NewParentState(worldState)
	for i, spec := range t.GetExecSpecs() {
		specPeerID, err := spec.ParsePeerID()
		if err != nil {
			return errors.Wrapf(err, "exec_specs[%d]", i)
		}

		execObjKey := forge_pass.BuildPassExecutionObjKey(objKey, specPeerID.String())
		if _, ok := skipExecs[spec.GetPeerId()]; !ok {
			skipExecs[spec.GetPeerId()] = struct{}{}
			_, err = forge_pass.CreateExecutionWithPass(
				ctx,
				worldState,
				sender,
				execObjKey,
				objKey,
				bcs,
				root,
				specPeerID,
			)
			if err == nil {
				// set the parent of the execution object to the pass
				err = parentState.SetObjectParent(ctx, execObjKey, objKey, true)
			}
			if err != nil {
				return errors.Wrapf(err, "exec_specs[%d]", i)
			}
		}

		// link the pass to the execution object
		err = worldState.SetGraphQuad(ctx, forge_pass.NewPassToExecutionQuad(objKey, execObjKey))
		if err != nil {
			return errors.Wrapf(err, "exec_specs[%d]", i)
		}
	}

	// apply changes
	bcs.SetBlock(root, true)

	// call update exec states to finalize
	updateSpecs := NewTxUpdateExecStatesTxn()
	return updateSpecs.ExecuteTx(ctx, worldState, sender, objKey, bcs, root)
}

// _ is a type assertion
var _ Transaction = ((*TxCreateExecSpecs)(nil))
