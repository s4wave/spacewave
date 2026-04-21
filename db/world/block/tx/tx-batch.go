package world_block_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxBatch constructs a new BATCH transaction.
//
// If there is 1 or less txs, returns that tx instead.
// Errors if there are no txs in the batch.
func NewTxBatch(txb *TxBatch) (*Tx, error) {
	if len(txb.GetTxs()) == 0 {
		return nil, block.ErrEmptyChanges
	}
	if len(txb.GetTxs()) == 1 {
		return txb.Txs[0], nil
	}
	return &Tx{
		TxType:  TxType_TxType_BATCH,
		TxBatch: txb,
	}, nil
}

// IsNil returns if the object is nil.
func (t *TxBatch) IsNil() bool {
	return t == nil
}

// GetTxType returns the type of transaction this is.
func (t *TxBatch) GetTxType() TxType {
	return TxType_TxType_BATCH
}

// GetEmpty checks if the tx is empty.
func (t *TxBatch) GetEmpty() bool {
	if len(t.GetTxs()) == 0 {
		return true
	}
	for _, tx := range t.GetTxs() {
		if empty, err := tx.GetEmpty(); empty || err != nil {
			return true
		}
	}
	return false
}

// Validate checks the execution tx type is in range.
func (t *TxBatch) Validate() error {
	for i, tx := range t.GetTxs() {
		err := func() error {
			empty, err := tx.GetEmpty()
			if err != nil {
				return err
			}
			if empty {
				return errors.New("empty transaction")
			}
			if err := tx.Validate(); err != nil {
				return err
			}
			return nil
		}()
		if err != nil {
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

// ExecuteTx executes the transaction against a world instance.
func (t *TxBatch) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	lookupWorldOp world.LookupOp,
	worldInstance world.WorldState,
) (sysErr bool, rerr error) {
	if err := t.Validate(); err != nil {
		return false, err
	}

	// apply sub-transactions.
	for i, tx := range t.GetTxs() {
		txo, err := tx.LocateTx()
		if err != nil {
			return false, errors.Wrapf(err, "tx_batch[%d]", i)
		}
		sysErr, rerr = txo.ExecuteTx(ctx, sender, lookupWorldOp, worldInstance)
		if rerr != nil {
			return sysErr, errors.Wrapf(rerr, "tx_batch[%d]", i)
		}
	}

	return false, nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (t *TxBatch) MarshalBlock() ([]byte, error) {
	return t.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (t *TxBatch) UnmarshalBlock(data []byte) error {
	return t.UnmarshalVT(data)
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
	_ Transaction              = ((*TxBatch)(nil))
	_ block.Block              = ((*TxBatch)(nil))
	_ block.BlockWithSubBlocks = ((*TxBatch)(nil))
)
