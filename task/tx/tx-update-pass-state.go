package task_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_task "github.com/aperturerobotics/forge/task"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// NewTxUpdatePassState constructs a new UPDATE_PASS_STATE transaction.
func NewTxUpdatePassState(objKey string) *Tx {
	return &Tx{
		TaskObjectKey: objKey,

		TxType:            TxType_TxType_UPDATE_PASS_STATE,
		TxUpdatePassState: &TxUpdatePassState{},
	}
}

// NewTxUpdatePassStateTxn constructs a new UPDATE_PASS_STATE transaction.
func NewTxUpdatePassStateTxn() Transaction {
	return &TxUpdatePassState{}
}

// GetTxType returns the type of transaction this is.
func (t *TxUpdatePassState) GetTxType() TxType {
	return TxType_TxType_UPDATE_PASS_STATE
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxUpdatePassState) Validate() error {
	return nil
}

// ExecuteTx executes the transaction against the Task instance.
func (t *TxUpdatePassState) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	executorPeerID peer.ID,
	objKey string,
	bcs *block.Cursor,
	root *forge_task.Task,
) error {
	// ensure RUNNING state
	err := root.GetTaskState().EnsureMatches(forge_task.State_TaskState_RUNNING)
	if err != nil {
		return err
	}

	// lookup the latest Pass
	// TODO

	// mark as changed
	bcs.SetBlock(root, true)

	return errors.New("TODO update pass state")

	// check the number of completed / failed states
	/*
		var nsuccess, nfailed int
		var failErr error
		for _, execState := range execStates {
			execResult := execState.GetResult()
			if !execResult.IsEmpty() {
				if execResult.IsSuccessful() {
					nsuccess++
				} else {
					nfailed++
					if failErr == nil {
						failErrStr := execResult.GetFailError()
						if failErrStr != "" {
							failErr = errors.New(failErrStr)
						}
					}
				}
			}
		}

		// transition to the COMPLETE state with an error if all executions errored
		if nfailed == nstates {
			if failErr == nil {
				failErr = errors.New("unknown errors")
			}
			failErr = errors.Wrap(failErr, "execution failed")

			root.PassState = forge_pass.State_PassState_COMPLETE
			root.Result = forge_value.NewResultWithError(failErr)
			return nil
		}

		// transition to the CHECKING state if the Pass has succeeded
		replicas := int(root.GetReplicas())
		if nsuccess >= replicas {
			root.PassState = forge_pass.State_PassState_CHECKING

			// also remove any unsuccessful exec states from the list
			for i := 0; i < len(execStates); i++ {
				if !execStates[i].GetResult().IsSuccessful() {
					execStates = append(execStates[:i], execStates[i+1:]...)
					i--
				}
			}
		}
	*/
}

// _ is a type assertion
var _ Transaction = ((*TxUpdatePassState)(nil))
