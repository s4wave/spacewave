package pass_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_pass "github.com/aperturerobotics/forge/pass"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/aperturerobotics/hydra/world"
	proto "github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// WorldOperationTypeID is the transaction object operation type id.
// Corresponds to a single *TransactionData object.
var WorldOperationTypeID = "forge/pass/tx"

// Transaction is an instance of a transaction object.
type Transaction interface {
	proto.Message

	// GetTxType returns the type of transaction this is.
	GetTxType() TxType
	// Validate performs a cursory check of the transaction.
	// Note: this should not fetch network data.
	Validate() error
	// ExecuteTx executes the transaction against the execution instance.
	// bcs is located at the pass state root.
	// The result is written into bcs.
	ExecuteTx(
		ctx context.Context,
		worldState world.WorldState,
		executorPeerID peer.ID,
		bcs *block.Cursor,
		root *forge_pass.Pass,
	) error
}

// Validate checks the transaction (cursory checks only)
func (t *Tx) Validate() error {
	if len(t.GetPassObjectKey()) == 0 {
		return errors.Wrap(world.ErrEmptyObjectKey, "pass_object_key")
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
	case TxType_TxType_START:
		return nil
	case TxType_TxType_EXEC_COMPLETE:
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
	case TxType_TxType_EXEC_COMPLETE:
		return t.GetTxExecComplete(), nil
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

	_, _, err = world.AccessWorldObject(ctx, worldHandle, t.GetPassObjectKey(), true, func(bcs *block.Cursor) error {
		ps, err := forge_pass.UnmarshalPass(bcs)
		if err != nil {
			return err
		}
		return ttx.ExecuteTx(ctx, worldHandle, sender, bcs, ps)
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
	return proto.Marshal(t)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (t *Tx) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, t)
}

// _ is a type assertion
var _ world.Operation = ((*Tx)(nil))
