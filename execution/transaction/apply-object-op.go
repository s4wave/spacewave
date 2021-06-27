package execution_transaction

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_execution "github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
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
	// convert op from a ByteSlice to a TransactionData (if necessary)
	executionTxData, err := ByteSliceToTransactionData(op)
	if err != nil {
		return false, errors.Wrap(err, "parse operation to execution tx")
	}
	if err := executionTxData.GetExecutionTxType().Validate(); err != nil {
		return false, err
	}

	tx, err := executionTxData.UnmarshalTransaction()
	if err != nil {
		return false, err
	}

	var nrootRef *block.BlockRef
	err = objectHandle.AccessWorldState(ctx, true, nil, func(bls *bucket_lookup.Cursor) error {
		btx, bcs := bls.BuildTransaction(nil)
		ex, err := forge_execution.UnmarshalExecution(bcs)
		if err != nil {
			return err
		}
		err = tx.ExecuteTx(ctx, opSender, bcs, ex)
		if err != nil {
			return err
		}
		nrootRef, bcs, err = btx.Write(true)
		if err != nil {
			return err
		}
		return err
	})
	if err != nil {
		return false, err
	}

	_, err = objectHandle.SetRootRef(&bucket.ObjectRef{RootRef: nrootRef})
	return true, err
}

// _ is a type assertion
var _ world.ApplyObjectOpFunc = ApplyObjectOp
