package pass_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_pass "github.com/aperturerobotics/forge/pass"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// ApplyWorldOp applies the transaction as a world op.
func ApplyWorldOp(
	ctx context.Context,
	worldState world.WorldState,
	operationTypeID string,
	op world.Operation,
	opSender peer.ID,
) (handled bool, err error) {
	// convert op from a ByteSlice to a Tx (if necessary)
	txd, err := ByteSliceToTx(op)
	if err != nil {
		return false, errors.Wrap(err, "parse operation to execution tx")
	}
	if err := txd.Validate(); err != nil {
		return false, err
	}
	tx, err := txd.LocateTx()
	if err != nil {
		return false, err
	}

	objectHandle, err := world.MustGetObject(worldState, txd.GetPassObjectKey())
	if err != nil {
		return false, err
	}
	nrootRef, err := world.AccessObject(ctx, objectHandle.AccessWorldState, nil, func(bcs *block.Cursor) error {
		ps, err := forge_pass.UnmarshalPass(bcs)
		if err != nil {
			return err
		}
		return tx.ExecuteTx(ctx, worldState, opSender, bcs, ps)
	})
	if err != nil {
		return false, err
	}

	_, err = objectHandle.SetRootRef(nrootRef)
	return true, err
}

// _ is a type assertion
var _ world.ApplyWorldOpFunc = ApplyWorldOp
