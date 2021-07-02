package execution_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_execution "github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// ApplyObjectOp applies the transaction as an object op.
func ApplyObjectOp(
	ctx context.Context,
	objectHandle world.ObjectState,
	operationTypeID string,
	op world.Operation,
	opSender peer.ID,
) (handled bool, err error) {
	// convert op from a ByteSlice to a Tx (if necessary)
	executionTxData, err := ByteSliceToTx(op)
	if err != nil {
		return false, errors.Wrap(err, "parse operation to execution tx")
	}
	if err := executionTxData.GetTxType().Validate(); err != nil {
		return false, err
	}

	tx, err := executionTxData.LocateTx()
	if err != nil {
		return false, err
	}

	nrootRef, err := world.AccessObject(ctx, objectHandle.AccessWorldState, nil, func(bcs *block.Cursor) error {
		ex, err := forge_execution.UnmarshalExecution(bcs)
		if err != nil {
			return err
		}
		return tx.ExecuteTx(ctx, opSender, bcs, ex)
	})
	if err != nil {
		return false, err
	}

	_, err = objectHandle.SetRootRef(nrootRef)
	return true, err
}

// _ is a type assertion
var _ world.ApplyObjectOpFunc = ApplyObjectOp
