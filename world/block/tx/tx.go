package world_block_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
	proto "google.golang.org/protobuf/proto"
)

// Transaction is an instance of a transaction object.
type Transaction interface {
	proto.Message

	// GetTxType returns the type of transaction this is.
	GetTxType() TxType
	// GetEmpty checks if the tx is empty.
	GetEmpty() bool
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
	case TxType_TxType_APPLY_WORLD_OP:
	case TxType_TxType_CREATE_OBJECT:
	case TxType_TxType_OBJECT_SET:
	case TxType_TxType_OBJECT_INC_REV:
	case TxType_TxType_DELETE_OBJECT:
	case TxType_TxType_SET_GRAPH_QUAD:
	case TxType_TxType_DELETE_GRAPH_QUAD:
	case TxType_TxType_BATCH:
	default:
		return errors.Wrap(world.ErrUnhandledOp, t.String())
	}
	return nil
}

// Clone clones the tx object.
func (t *Tx) Clone() *Tx {
	if t == nil {
		return nil
	}
	return &Tx{
		TxType: t.GetTxType(),

		TxApplyObjectOp:   t.GetTxApplyObjectOp().Clone(),
		TxApplyWorldOp:    t.GetTxApplyWorldOp().Clone(),
		TxCreateObject:    t.GetTxCreateObject().Clone(),
		TxObjectIncRev:    t.GetTxObjectIncRev().Clone(),
		TxObjectSet:       t.GetTxObjectSet().Clone(),
		TxDeleteObject:    t.GetTxDeleteObject().Clone(),
		TxSetGraphQuad:    t.GetTxSetGraphQuad().Clone(),
		TxDeleteGraphQuad: t.GetTxDeleteGraphQuad().Clone(),
		TxBatch:           t.GetTxBatch().Clone(),
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

// GetEmpty checks if the tx is empty.
func (t *Tx) GetEmpty() (bool, error) {
	if t.GetTxType() == 0 {
		return true, nil
	}

	btx, err := t.LocateTx()
	if err != nil {
		return false, err
	}
	return btx.GetEmpty(), nil
}

// LocateTx returns the sub-block for the transaction.
func (t *Tx) LocateTx() (Transaction, error) {
	switch t.GetTxType() {
	case TxType_TxType_APPLY_OBJECT_OP:
		return t.GetTxApplyObjectOp(), nil
	case TxType_TxType_APPLY_WORLD_OP:
		return t.GetTxApplyWorldOp(), nil
	case TxType_TxType_CREATE_OBJECT:
		return t.GetTxCreateObject(), nil
	case TxType_TxType_OBJECT_SET:
		return t.GetTxObjectSet(), nil
	case TxType_TxType_OBJECT_INC_REV:
		return t.GetTxObjectIncRev(), nil
	case TxType_TxType_DELETE_OBJECT:
		return t.GetTxDeleteObject(), nil
	case TxType_TxType_SET_GRAPH_QUAD:
		return t.GetTxSetGraphQuad(), nil
	case TxType_TxType_DELETE_GRAPH_QUAD:
		return t.GetTxDeleteGraphQuad(), nil
	case TxType_TxType_BATCH:
		return t.GetTxBatch(), nil
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
		m[1] = t.GetTxApplyWorldOp()
	case TxType_TxType_APPLY_OBJECT_OP:
		m[2] = t.GetTxApplyObjectOp()
	case TxType_TxType_CREATE_OBJECT:
		m[3] = t.GetTxCreateObject()
	case TxType_TxType_OBJECT_SET:
		m[4] = t.GetTxObjectSet()
	case TxType_TxType_OBJECT_INC_REV:
		m[5] = t.GetTxObjectIncRev()
	case TxType_TxType_DELETE_OBJECT:
		m[6] = t.GetTxDeleteObject()
	case TxType_TxType_SET_GRAPH_QUAD:
		m[7] = t.GetTxSetGraphQuad()
	case TxType_TxType_DELETE_GRAPH_QUAD:
		m[8] = t.GetTxDeleteGraphQuad()
	case TxType_TxType_BATCH:
		m[9] = t.GetTxBatch()
	}
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (t *Tx) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			v := t.GetTxApplyWorldOp()
			if v == nil && create {
				v = &TxApplyWorldOp{}
				t.TxApplyWorldOp = v
			}
			return v
		}
	case 2:
		return func(create bool) block.SubBlock {
			v := t.GetTxApplyObjectOp()
			if v == nil && create {
				v = &TxApplyObjectOp{}
				t.TxApplyObjectOp = v
			}
			return v
		}
	case 3:
		return func(create bool) block.SubBlock {
			v := t.GetTxCreateObject()
			if v == nil && create {
				v = &TxCreateObject{}
				t.TxCreateObject = v
			}
			return v
		}
	case 4:
		return func(create bool) block.SubBlock {
			v := t.GetTxObjectSet()
			if v == nil && create {
				v = &TxObjectSet{}
				t.TxObjectSet = v
			}
			return v
		}
	case 5:
		return func(create bool) block.SubBlock {
			v := t.GetTxObjectIncRev()
			if v == nil && create {
				v = &TxObjectIncRev{}
				t.TxObjectIncRev = v
			}
			return v
		}
	case 6:
		return func(create bool) block.SubBlock {
			v := t.GetTxDeleteObject()
			if v == nil && create {
				v = &TxDeleteObject{}
				t.TxDeleteObject = v
			}
			return v
		}
	case 9:
		return func(create bool) block.SubBlock {
			v := t.GetTxBatch()
			if v == nil && create {
				v = &TxBatch{}
				t.TxBatch = v
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
