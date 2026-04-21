package task_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	"github.com/s4wave/spacewave/db/world"
	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// WorldOperationTypeID is the transaction object operation type id.
// Corresponds to a single *TransactionData object.
var WorldOperationTypeID = "forge/task/tx"

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
	// ExecuteTx executes the transaction against the task instance.
	// bcs is located at the task state root.
	// The result is written into bcs.
	ExecuteTx(
		ctx context.Context,
		worldState world.WorldState,
		sender peer.ID,
		objKey string,
		bcs *block.Cursor,
		root *forge_task.Task,
	) error
}

// Validate checks the transaction (cursory checks only)
func (t *Tx) Validate() error {
	if len(t.GetTaskObjectKey()) == 0 {
		return errors.Wrap(world.ErrEmptyObjectKey, "task_object_key")
	}
	if err := t.GetTxType().Validate(); err != nil {
		return err
	}
	ttx, err := t.LocateTx()
	if err != nil {
		return err
	}
	return ttx.Validate()
}

// Validate checks the execution tx type is in range.
func (t TxType) Validate() error {
	switch t {
	case TxType_TxType_UPDATE_INPUTS:
		return nil
	case TxType_TxType_START:
		return nil
	case TxType_TxType_UPDATE_WITH_PASS_STATE:
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
	case TxType_TxType_UPDATE_INPUTS:
		return t.GetTxUpdateInputs(), nil
	case TxType_TxType_START:
		return t.GetTxStart(), nil
	case TxType_TxType_UPDATE_WITH_PASS_STATE:
		return t.GetTxUpdateWithPassState(), nil
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
	return WorldOperationTypeID
}

// ApplyWorldOp applies the operation as a world operation.
func (t *Tx) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	ttx, err := t.LocateTx()
	if err != nil {
		return false, err
	}

	objKey := t.GetTaskObjectKey()
	_, _, err = world.AccessWorldObject(ctx, worldHandle, objKey, true, func(bcs *block.Cursor) error {
		ps, err := forge_task.UnmarshalTask(ctx, bcs)
		if err != nil {
			return err
		}
		err = ttx.ExecuteTx(ctx, worldHandle, sender, objKey, bcs, ps)
		if err != nil {
			return err
		}
		bcs.SetPreWriteHook(func(b any) error {
			v, vOk := b.(*forge_task.Task)
			if !vOk {
				return block.ErrUnexpectedType
			}
			return v.Validate()
		})
		return nil
	})
	return false, err
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (t *Tx) ApplyWorldObjectOp(ctx context.Context, le *logrus.Entry, objectHandle world.ObjectState, sender peer.ID) (sysErr bool, err error) {
	// world operation only
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (t *Tx) MarshalBlock() ([]byte, error) {
	return t.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (t *Tx) UnmarshalBlock(data []byte) error {
	return t.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*Tx)(nil))
