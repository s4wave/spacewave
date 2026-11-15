package pass_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	"github.com/s4wave/spacewave/db/world"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Transaction is an instance of a transaction object.
type Transaction interface {
	// MarshalVT marshals to binary.
	MarshalVT() ([]byte, error)
	// UnmarshalVT unmarshals from binary.
	UnmarshalVT(data []byte) error

	// GetTxType returns the type of transaction this is.
	GetTxType() TxType
	// Validate performs a cursory check of the transaction.
	// Note: this should not fetch network data.
	Validate() error
	// ExecuteTx executes the transaction against the pass instance.
	// bcs is located at the pass state root.
	// note: bcs is sometimes not set, check to be sure.
	// The result is written into bcs.
	ExecuteTx(
		ctx context.Context,
		worldState world.WorldState,
		sender peer.ID,
		objKey string,
		bcs *block.Cursor,
		root *forge_pass.Pass,
	) error
}

// Validate performs cursory checks of the transaction.
func (t *Tx) Validate() error {
	// Require a pass object key and a supported transaction type.
	if len(t.GetPassObjectKey()) == 0 {
		return errors.Wrap(world.ErrEmptyObjectKey, "pass_object_key")
	}
	if err := t.GetTxType().Validate(); err != nil {
		return err
	}

	// Locate the pass transaction payload for validation.
	ttx, err := t.LocateTx()
	if err != nil {
		return err
	}

	return ttx.Validate()
}

// Validate checks the execution tx type is in range.
func (t TxType) Validate() error {
	switch t {
	case TxType_TxType_START:
		return nil
	case TxType_TxType_CREATE_EXEC_SPECS:
		return nil
	case TxType_TxType_UPDATE_EXEC_STATES:
		return nil
	case TxType_TxType_COMPLETE:
		return nil
	case TxType_TxType_CANCEL:
		return nil
	default:
		return errors.Wrap(world.ErrUnhandledOp, t.String())
	}
}

// LocateTx returns the sub-block for the transaction.
func (t *Tx) LocateTx() (Transaction, error) {
	switch t.GetTxType() {
	case TxType_TxType_START:
		return t.GetTxStart(), nil
	case TxType_TxType_CREATE_EXEC_SPECS:
		return t.GetTxCreateExecSpecs(), nil
	case TxType_TxType_UPDATE_EXEC_STATES:
		return t.GetTxUpdateExecStates(), nil
	case TxType_TxType_COMPLETE:
		return t.GetTxComplete(), nil
	case TxType_TxType_CANCEL:
		return t.GetTxCancel(), nil
	default:
		return nil, errors.Wrap(world.ErrUnhandledOp, t.String())
	}
}

// ByteSliceToTx converts a byte slice block a Tx.
// If blk is nil, returns nil, nil
// If the blk is already parsed to a Tx, returns the Tx.
func ByteSliceToTx(blk block.Block) (*Tx, error) {
	// Preserve an absent pass transaction block.
	if blk == nil {
		return nil, nil
	}

	// Decode a byte slice containing a pass transaction.
	var out *Tx
	nr, ok := blk.(*byteslice.ByteSlice)
	if ok && nr != nil {
		out = &Tx{}
		if err := out.UnmarshalVT(nr.GetBytes()); err != nil {
			return nil, err
		}
		return out, nil
	}

	// Require a decoded pass transaction for other block representations.
	out, ok = blk.(*Tx)
	if !ok {
		return out, block.ErrUnexpectedType
	}

	return out, nil
}

// ApplyWorldOp applies the operation as a world operation.
func (t *Tx) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Locate the pass transaction payload to apply.
	ttx, err := t.LocateTx()
	if err != nil {
		return false, err
	}

	// Apply the transaction within access to the pass World object.
	objKey := t.GetPassObjectKey()
	_, _, err = world.AccessWorldObject(ctx, worldHandle, objKey, true, func(bcs *block.Cursor) error {
		// Decode the current pass state from the World object.
		ps, err := forge_pass.UnmarshalPass(ctx, bcs)
		if err != nil {
			return err
		}

		// Execute the pass transaction and validate the resulting state.
		err = ttx.ExecuteTx(ctx, worldHandle, sender, objKey, bcs, ps)
		if err == nil {
			err = world.NewOpRefusedError(ps.Validate(true))
		}

		return err
	})

	return false, err
}

// _ is a type assertion
var _ world.Operation = (*Tx)(nil)
