package pass_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_pass "github.com/aperturerobotics/forge/pass"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// NewTxUpdateExecStates constructs a new UPDATE_EXEC_STATES transaction.
func NewTxUpdateExecStates(objKey string) *Tx {
	return &Tx{
		PassObjectKey: objKey,

		TxType:             TxType_TxType_UPDATE_EXEC_STATES,
		TxUpdateExecStates: &TxUpdateExecStates{},
	}
}

// NewTxUpdateExecStatesTxn constructs a new UPDATE_EXEC_STATES transaction.
func NewTxUpdateExecStatesTxn() Transaction {
	return &TxUpdateExecStates{}
}

// GetTxType returns the type of transaction this is.
func (t *TxUpdateExecStates) GetTxType() TxType {
	return TxType_TxType_UPDATE_EXEC_STATES
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxUpdateExecStates) Validate() error {
	return nil
}

// ExecuteTx executes the transaction against the pass instance.
func (t *TxUpdateExecStates) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	executorPeerID peer.ID,
	objKey string,
	bcs *block.Cursor,
	root *forge_pass.Pass,
) error {
	// ensure RUNNING state
	err := root.GetPassState().EnsureMatches(forge_pass.State_PassState_RUNNING)
	if err != nil {
		return err
	}

	// collect all attached executions
	execObjs, execObjKeys, err := forge_pass.CollectPassExecutions(ctx, worldState, objKey)
	if err != nil {
		return err
	}

	// copy to the states slice & validate
	err = root.ApplyExecStates(bcs, execObjKeys, execObjs)
	if err != nil {
		return err
	}

	// mark as changed
	bcs.SetBlock(root, true)

	// if there are no exec states, stop here.
	execStates := root.GetExecStates()
	nstates := len(execStates)
	if nstates == 0 {
		return nil
	}

	// check the number of completed / failed states
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

	// transition to the CHECKING state if enough executions have succeeded
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

	return nil
}

// _ is a type assertion
var _ Transaction = ((*TxUpdateExecStates)(nil))
