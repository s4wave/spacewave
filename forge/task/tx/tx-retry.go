package task_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxRetry constructs a RETRY transaction for a failed Pass.
func NewTxRetry(objKey string, expectedPassNonce uint64, nextInputs *forge_target.ValueSet) *Tx {
	return &Tx{
		TaskObjectKey: objKey,
		TxType:        TxType_TxType_RETRY,
		TxRetry: &TxRetry{
			ExpectedPassNonce: expectedPassNonce,
			NextInputs:        nextInputs,
		},
	}
}

// NewTxRetryTxn constructs a new RETRY transaction.
func NewTxRetryTxn() Transaction {
	return &TxRetry{}
}

// GetTxType returns the type of transaction this is.
func (t *TxRetry) GetTxType() TxType {
	return TxType_TxType_RETRY
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxRetry) Validate() error {
	nextInputs := t.GetNextInputs()
	if nextInputs == nil {
		return nil
	}
	if len(nextInputs.GetOutputs()) != 0 {
		return errors.New("next_inputs: outputs: must be empty")
	}
	if err := nextInputs.Validate(); err != nil {
		return errors.Wrap(err, "next_inputs")
	}
	return nil
}

// ExecuteTx executes the transaction against the Task instance.
func (t *TxRetry) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	sender peer.ID,
	objKey string,
	bcs *block.Cursor,
	root *forge_task.Task,
) error {
	if root == nil {
		return errors.New("unexpected empty root task object")
	}
	if err := t.Validate(); err != nil {
		return err
	}
	nextInputs := t.GetNextInputs()
	if nextInputs == nil {
		nextInputs = forge_target.NewValueSet()
	}

	expectedNonce := t.GetExpectedPassNonce()
	passes, _, passKeys, err := forge_task.CollectTaskPasses(ctx, worldState, objKey)
	if err != nil {
		return err
	}

	var predecessor *forge_pass.Pass
	var predecessorKey string
	for i, pass := range passes {
		if pass.GetPassNonce() > expectedNonce {
			return errors.Errorf("pass %d already has successor", expectedNonce)
		}
		if pass.GetPassNonce() == expectedNonce {
			predecessor = pass
			predecessorKey = passKeys[i]
		}
	}
	if root.GetPassNonce() != expectedNonce {
		return errors.Errorf("task points at pass %d, expected %d", root.GetPassNonce(), expectedNonce)
	}
	if predecessor == nil || predecessorKey == "" {
		return errors.Errorf("failed pass %d is not linked to task", expectedNonce)
	}

	if predecessor.GetPassState() != forge_pass.State_PassState_COMPLETE {
		return errors.Errorf("pass %d is not terminal", expectedNonce)
	}
	if predecessor.GetResult() == nil || predecessor.GetResult().GetSuccess() {
		return errors.Errorf("pass %d did not fail", expectedNonce)
	}

	execObjs, _, err := forge_pass.CollectPassExecutions(ctx, worldState, predecessorKey)
	if err != nil {
		return err
	}
	for i, exec := range execObjs {
		if exec.GetExecutionState() != forge_execution.State_ExecutionState_COMPLETE {
			return errors.Errorf("pass %d execution %d is still live", expectedNonce, i)
		}
	}
	// A replay after the transition is a no-op only for the same named inputs.
	if root.GetTaskState() == forge_task.State_TaskState_PENDING {
		if sameInputs(root.GetValueSet().GetInputs(), nextInputs.GetInputs()) {
			return nil
		}
		return errors.New("retry already applied with different inputs")
	}

	valueSet := root.GetValueSet()
	if valueSet == nil {
		valueSet = forge_target.NewValueSet()
	} else {
		valueSet = valueSet.Clone()
	}
	nextInputs = nextInputs.Clone()
	valueSet.Inputs = nextInputs.GetInputs()
	valueSet.Outputs = nil
	valueSet.SortValues()
	root.ValueSet = valueSet
	root.TaskState = forge_task.State_TaskState_PENDING
	bcs.SetBlock(root, true)
	return nil
}

func sameInputs(a, b forge_value.ValueSlice) bool {
	added, removed, changed := a.Compare(b)
	return len(added) == 0 && len(removed) == 0 && len(changed) == 0
}

// _ is a type assertion
var _ Transaction = (*TxRetry)(nil)
