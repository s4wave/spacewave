package execution_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_execution "github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/aperturerobotics/hydra/world"
	proto "github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ObjectOperationTypeID is the transaction object operation type id.
var ObjectOperationTypeID = "forge/execution/tx"

// LookupWorldOp performs the lookup operation for the pass op types.
func LookupWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID == ObjectOperationTypeID {
		return &Tx{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.LookupOp = LookupWorldOp

// Transaction is an instance of a transaction object.
type Transaction interface {
	proto.Message

	// GetTxType returns the type of transaction this is.
	GetTxType() TxType
	// Validate performs a cursory check of the transaction.
	// Note: this should not fetch network data.
	Validate() error
	// ExecuteTx executes the transaction against the execution instance.
	// exCursor should be located at the execution state root.
	// The result is written into exCursor.
	ExecuteTx(
		ctx context.Context,
		sender peer.ID,
		exCursor *block.Cursor,
		root *forge_execution.Execution,
	) error
}

// Validate checks the tx.
func (t *Tx) Validate() error {
	ttx, err := t.LocateTx()
	if err != nil {
		return err
	}
	if err := ttx.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate checks the execution tx type is in range.
func (t TxType) Validate() error {
	switch t {
	case TxType_TxType_START:
		return nil
	case TxType_TxType_SET_OUTPUTS:
		return nil
	case TxType_TxType_COMPLETE:
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
	case TxType_TxType_SET_OUTPUTS:
		return t.GetTxSetOutputs(), nil
	case TxType_TxType_COMPLETE:
		return t.GetTxComplete(), nil
	default:
		return nil, errors.Wrap(world.ErrUnhandledOp, t.String())
	}
}

// ByteSliceToTx converts a byte slice block a Tx.
// If blk is nil, returns nil, nil
// If the blk is already parsed to a MockWorldOp, returns the MockWorldOp.
func ByteSliceToTx(blk block.Block) (*Tx, error) {
	if blk == nil {
		return nil, nil
	}
	var out *Tx
	nr, ok := blk.(*byteslice.ByteSlice)
	if ok && nr != nil {
		out = &Tx{}
		if err := out.UnmarshalBlock(nr.GetBytes()); err != nil {
			return nil, err
		}
		return out, nil
	}
	out, ok = blk.(*Tx)
	if !ok {
		return out, block.ErrUnexpectedType
	}
	return out, nil
}

// GetOperationTypeId returns the operation type identifier.
func (t *Tx) GetOperationTypeId() string {
	return ObjectOperationTypeID
}

// ApplyWorldOp applies the operation as a world operation.
// returns false, ErrUnhandledOp if the operation cannot handle a world op
func (t *Tx) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (t *Tx) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := t.GetTxType().Validate(); err != nil {
		return false, err
	}

	tx, err := t.LocateTx()
	if err != nil {
		return false, err
	}

	// access & update the execution object
	_, _, err = world.AccessObjectState(ctx, objectHandle, true, func(bcs *block.Cursor) error {
		ex, err := forge_execution.UnmarshalExecution(bcs)
		if err != nil {
			return err
		}
		err = tx.ExecuteTx(ctx, sender, bcs, ex)
		if err == nil {
			err = ex.Validate()
		}
		return err
	})
	return false, err
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (t *Tx) MarshalBlock() ([]byte, error) {
	return proto.Marshal(t)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (t *Tx) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, t)
}

// _ is a type assertion
var _ world.Operation = ((*Tx)(nil))
