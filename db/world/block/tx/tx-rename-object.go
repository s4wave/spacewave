package world_block_tx

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxRenameObject constructs a new RENAME_OBJECT transaction.
func NewTxRenameObject(oldKey, newKey string) (*Tx, error) {
	return &Tx{
		TxType: TxType_TxType_RENAME_OBJECT,
		TxRenameObject: &TxRenameObject{
			OldObjectKey: oldKey,
			NewObjectKey: newKey,
		},
	}, nil
}

// NewTxRenameObjectTxn constructs a new RENAME_OBJECT transaction.
func NewTxRenameObjectTxn() Transaction {
	return &TxRenameObject{}
}

// IsNil checks if the object is nil.
func (t *TxRenameObject) IsNil() bool {
	return t == nil
}

// GetTxType returns the type of transaction this is.
func (t *TxRenameObject) GetTxType() TxType {
	return TxType_TxType_RENAME_OBJECT
}

// GetEmpty checks if the tx is empty.
func (t *TxRenameObject) GetEmpty() bool {
	return t.GetOldObjectKey() == "" && t.GetNewObjectKey() == ""
}

// Clone clones the tx object.
func (t *TxRenameObject) Clone() *TxRenameObject {
	if t == nil {
		return nil
	}
	return &TxRenameObject{
		OldObjectKey: t.GetOldObjectKey(),
		NewObjectKey: t.GetNewObjectKey(),
	}
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxRenameObject) Validate() error {
	if len(t.GetOldObjectKey()) == 0 || len(t.GetNewObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	return nil
}

// ExecuteTx executes the transaction against a world instance.
func (t *TxRenameObject) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	lookupWorldOp world.LookupOp,
	worldInstance world.WorldState,
) (sysErr bool, rerr error) {
	if err := t.Validate(); err != nil {
		return false, err
	}

	_, err := worldInstance.RenameObject(ctx, t.GetOldObjectKey(), t.GetNewObjectKey(), false)
	return false, err
}

// _ is a type assertion
var _ Transaction = ((*TxRenameObject)(nil))
