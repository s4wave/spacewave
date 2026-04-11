package world_block_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// Transaction is an instance of a transaction object.
type Transaction interface {
	// MarshalVT marshals the transaction to binary.
	MarshalVT() ([]byte, error)
	// UnmarshalVT unmarshals the transaction from binary.
	UnmarshalVT(data []byte) error

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
	case TxType_TxType_GC_SWEEP:
	default:
		return errors.Wrap(world.ErrUnhandledOp, t.String())
	}
	return nil
}

// IsNil checks if the object is nil.
func (t *Tx) IsNil() bool {
	return t == nil
}

// Clone clones the tx object.
func (t *Tx) Clone() *Tx {
	return t.CloneVT()
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
	case TxType_TxType_GC_SWEEP:
		return t.GetTxGcSweep(), nil
	default:
		return nil, errors.Wrap(world.ErrUnhandledOp, t.GetTxType().String())
	}
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

// ApplySubBlock applies a sub-block change with a field id.
func (t *Tx) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 2:
		return block.ApplySubBlock(&t.TxApplyWorldOp, next)
	case 3:
		return block.ApplySubBlock(&t.TxApplyObjectOp, next)
	case 4:
		return block.ApplySubBlock(&t.TxCreateObject, next)
	case 5:
		return block.ApplySubBlock(&t.TxObjectSet, next)
	case 6:
		return block.ApplySubBlock(&t.TxObjectIncRev, next)
	case 7:
		return block.ApplySubBlock(&t.TxDeleteObject, next)
	case 8:
		return block.ApplySubBlock(&t.TxSetGraphQuad, next)
	case 9:
		return block.ApplySubBlock(&t.TxDeleteGraphQuad, next)
	case 10:
		return block.ApplySubBlock(&t.TxBatch, next)
	case 11:
		return block.ApplySubBlock(&t.TxGcSweep, next)
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
	case TxType_TxType_CREATE_OBJECT:
		m[4] = t.GetTxCreateObject()
	case TxType_TxType_OBJECT_SET:
		m[5] = t.GetTxObjectSet()
	case TxType_TxType_OBJECT_INC_REV:
		m[6] = t.GetTxObjectIncRev()
	case TxType_TxType_DELETE_OBJECT:
		m[7] = t.GetTxDeleteObject()
	case TxType_TxType_SET_GRAPH_QUAD:
		m[8] = t.GetTxSetGraphQuad()
	case TxType_TxType_DELETE_GRAPH_QUAD:
		m[9] = t.GetTxDeleteGraphQuad()
	case TxType_TxType_BATCH:
		m[10] = t.GetTxBatch()
	case TxType_TxType_GC_SWEEP:
		m[11] = t.GetTxGcSweep()
	}
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (t *Tx) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 2:
		return block.NewSubBlockCtor(&t.TxApplyWorldOp, func() *TxApplyWorldOp { return &TxApplyWorldOp{} })
	case 3:
		return block.NewSubBlockCtor(&t.TxApplyObjectOp, func() *TxApplyObjectOp { return &TxApplyObjectOp{} })
	case 4:
		return block.NewSubBlockCtor(&t.TxCreateObject, func() *TxCreateObject { return &TxCreateObject{} })
	case 5:
		return block.NewSubBlockCtor(&t.TxObjectSet, func() *TxObjectSet { return &TxObjectSet{} })
	case 6:
		return block.NewSubBlockCtor(&t.TxObjectIncRev, func() *TxObjectIncRev { return &TxObjectIncRev{} })
	case 7:
		return block.NewSubBlockCtor(&t.TxDeleteObject, func() *TxDeleteObject { return &TxDeleteObject{} })
	case 8:
		return block.NewSubBlockCtor(&t.TxSetGraphQuad, func() *TxSetGraphQuad { return &TxSetGraphQuad{} })
	case 9:
		return block.NewSubBlockCtor(&t.TxDeleteGraphQuad, func() *TxDeleteGraphQuad { return &TxDeleteGraphQuad{} })
	case 10:
		return block.NewSubBlockCtor(&t.TxBatch, func() *TxBatch { return &TxBatch{} })
	case 11:
		return block.NewSubBlockCtor(&t.TxGcSweep, func() *TxGCSweep { return &TxGCSweep{} })
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Tx)(nil))
	_ block.BlockWithSubBlocks = ((*Tx)(nil))
)
