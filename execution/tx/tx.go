package execution_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/aperturerobotics/hydra/world"
	proto "github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// ObjectOperationTypeID is the transaction object operation type id.
var ObjectOperationTypeID = "forge/execution/tx"

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
