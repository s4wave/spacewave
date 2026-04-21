package task_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	pass_tx "github.com/s4wave/spacewave/forge/pass/tx"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxStart constructs a new START transaction.
func NewTxStart(objKey string, assignSelf bool) *Tx {
	return &Tx{
		TaskObjectKey: objKey,

		TxType: TxType_TxType_START,
		TxStart: &TxStart{
			AssignSelf: assignSelf,
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
	return nil
}

// ExecuteTx executes the transaction against the pass instance.
func (t *TxStart) ExecuteTx(
	ctx context.Context,
	worldState world.WorldState,
	sender peer.ID,
	objKey string,
	bcs *block.Cursor,
	root *forge_task.Task,
) error {
	// ensure PENDING
	passState := root.GetTaskState()
	if passState != forge_task.State_TaskState_PENDING {
		return errors.Wrapf(
			forge_value.ErrUnknownState,
			"%s", passState.String(),
		)
	}

	// lookup the target
	taskTarget, _, err := root.FollowTargetRef(ctx, bcs)
	if err == nil {
		err = taskTarget.Validate()
	}
	if err != nil {
		if err != context.Canceled {
			err = errors.Wrap(err, "target")
		}
		return err
	}

	// cancel any existing Pass
	passes, _, passKeys, err := forge_task.CollectTaskPasses(ctx, worldState, objKey)
	if err != nil {
		return err
	}
	highestNonce := root.GetPassNonce()
	for i, pass := range passes {
		passKey := passKeys[i]
		if pass.GetPassState() != forge_pass.State_PassState_COMPLETE {
			passCompleteTx := pass_tx.NewTxComplete(
				passKey,
				forge_value.NewResultWithCanceled(errors.New("starting new pass")),
			)
			_, _, err = worldState.ApplyWorldOp(ctx, passCompleteTx, sender)
			if err != nil {
				return err
			}
		}
		if pn := pass.GetPassNonce(); pn > highestNonce {
			highestNonce = pn
		}
	}

	// promote to RUNNING
	root.TaskState = forge_task.State_TaskState_RUNNING

	// copy the value set from the pass.
	taskValueSet := root.GetValueSet().Clone()

	// create the new Pass
	nextNonce := highestNonce + 1
	root.PassNonce = nextNonce

	var passPeerID peer.ID
	if t.GetAssignSelf() {
		passPeerID = sender
	}

	passKey := forge_task.NewPassKey(objKey, nextNonce)
	_, _, err = forge_pass.CreatePassWithTarget(
		ctx,
		worldState,
		sender,
		passKey,
		taskValueSet,
		taskTarget,
		nextNonce,
		root.GetReplicas(),
		passPeerID.String(),
		root.GetTimestamp(),
	)
	if err != nil {
		return err
	}

	// set the parent of the pass to the task
	err = world_parent.SetObjectParent(ctx, worldState, passKey, objKey, false)
	if err != nil {
		return err
	}

	// link the pass to the task
	err = worldState.SetGraphQuad(ctx, forge_task.NewTaskToPassQuad(objKey, passKey, nextNonce))
	if err != nil {
		return err
	}

	// mark as dirty
	bcs.SetBlock(root, true)
	return nil
}

// _ is a type assertion
var _ Transaction = ((*TxStart)(nil))
