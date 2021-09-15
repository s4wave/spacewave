package world_block_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	proto "github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// Transaction is an instance of a transaction object.
type Transaction interface {
	proto.Message

	// GetTxType returns the type of transaction this is.
	GetTxType() TxType
	// Validate performs a cursory check of the transaction.
	// Note: this should not fetch network data.
	Validate() error
	// ExecuteTx executes the transaction against a world instance.
	ExecuteTx(
		ctx context.Context,
		sender peer.ID,
		lookupOp world.LookupOp,
		worldInstance world.WorldState,
	) (sysErr bool, err error)
}

// Validate checks the execution tx type is in range.
func (t TxType) Validate() error {
	switch t {
	case TxType_TxType_APPLY_OBJECT_OP:
		return nil
	case TxType_TxType_APPLY_WORLD_OP:
		return nil
	default:
		return errors.Wrap(world.ErrUnhandledOp, t.String())
	}
}

// Clone clones the tx object.
func (t *Tx) Clone() *Tx {
	if t == nil {
		return nil
	}
	return &Tx{
		TxType:          t.GetTxType(),
		TxApplyWorldOp:  t.GetTxApplyWorldOp().Clone(),
		TxApplyObjectOp: t.GetTxApplyObjectOp().Clone(),
	}
}

// Validate performs cursory validation of the Tx.
func (t *Tx) Validate() error {
	if err := t.GetTxType().Validate(); err != nil {
		return err
	}
	tx, err := t.LocateTx()
	if err != nil {
		return err
	}
	return tx.Validate()
}

// LocateTx returns the sub-block for the transaction.
func (t *Tx) LocateTx() (Transaction, error) {
	switch t.GetTxType() {
	case TxType_TxType_APPLY_OBJECT_OP:
		return t.GetTxApplyObjectOp(), nil
	case TxType_TxType_APPLY_WORLD_OP:
		return t.GetTxApplyWorldOp(), nil
	default:
		return nil, errors.Wrap(world.ErrUnhandledOp, t.String())
	}
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

// ApplySubBlock applies a sub-block change with a field id.
func (t *Tx) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 2:
		v, ok := next.(*TxApplyWorldOp)
		if !ok {
			return block.ErrUnexpectedType
		}
		t.TxApplyWorldOp = v
	case 3:
		v, ok := next.(*TxApplyObjectOp)
		if !ok {
			return block.ErrUnexpectedType
		}
		t.TxApplyObjectOp = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
func (t *Tx) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	switch t.GetTxType() {
	case TxType_TxType_APPLY_WORLD_OP:
		m[2] = t.GetTxApplyWorldOp()
	case TxType_TxType_APPLY_OBJECT_OP:
		m[3] = t.GetTxApplyObjectOp()
	}
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (t *Tx) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 2:
		return func(create bool) block.SubBlock {
			v := t.GetTxApplyWorldOp()
			if v == nil && create {
				v = &TxApplyWorldOp{}
				t.TxApplyWorldOp = v
			}
			return v
		}
	case 3:
		return func(create bool) block.SubBlock {
			v := t.GetTxApplyObjectOp()
			if v == nil && create {
				v = &TxApplyObjectOp{}
				t.TxApplyObjectOp = v
			}
			return v
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Tx)(nil))
	_ block.BlockWithSubBlocks = ((*Tx)(nil))
)
