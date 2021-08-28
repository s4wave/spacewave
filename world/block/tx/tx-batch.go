package world_block_tx

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
	proto "github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// Validate checks the execution tx type is in range.
func (t *TxBatch) Validate() error {
	for i, tx := range t.GetTxs() {
		if err := tx.Validate(); err != nil {
			return errors.Wrapf(err, "txs[%d]", i)
		}
	}
	return nil
}

// Clone creates a full copy of the tx batch.
func (t *TxBatch) Clone() *TxBatch {
	if t == nil {
		return nil
	}
	txs := make([]*Tx, len(t.Txs))
	for i := range txs {
		txs[i] = t.Txs[i].Clone()
	}
	return &TxBatch{Txs: txs}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (t *TxBatch) MarshalBlock() ([]byte, error) {
	return proto.Marshal(t)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (t *TxBatch) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, t)
}

// ApplySubBlock applies a sub-block change with a field id.
func (t *TxBatch) ApplySubBlock(id uint32, next block.SubBlock) error {
	// no-op
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
func (t *TxBatch) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[1] = t.FollowTxSet(nil)
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (t *TxBatch) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			return t.FollowTxSet(nil)
		}
	}
	return nil
}

// FollowTxSet follows the tx set sub block.
//
// bcs can be nil
func (t *TxBatch) FollowTxSet(bcs *block.Cursor) *sbset.SubBlockSet {
	return newTxSetContainer(&t.Txs, bcs)
}

// _ is a type assertion
var (
	_ block.Block              = ((*TxBatch)(nil))
	_ block.BlockWithSubBlocks = ((*TxBatch)(nil))
)
