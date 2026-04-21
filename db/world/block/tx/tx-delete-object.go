package world_block_tx

import (
	"context"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/db/world"
)

// NewTxDeleteObject constructs a new DELETE_OBJECT transaction.
func NewTxDeleteObject(objKey string) (*Tx, error) {
	return &Tx{
		TxType: TxType_TxType_DELETE_OBJECT,
		TxDeleteObject: &TxDeleteObject{
			ObjectKey: objKey,
		},
	}, nil
}

// NewTxDeleteObjectTxn constructs a new DELETE_OBJECT transaction.
func NewTxDeleteObjectTxn() Transaction {
	return &TxDeleteObject{}
}

// IsNil checks if the object is nil.
func (t *TxDeleteObject) IsNil() bool {
	return t == nil
}

// GetTxType returns the type of transaction this is.
func (t *TxDeleteObject) GetTxType() TxType {
	return TxType_TxType_DELETE_OBJECT
}

// GetEmpty checks if the tx is empty.
func (t *TxDeleteObject) GetEmpty() bool {
	return t.GetObjectKey() == ""
}

// Clone clones the tx object.
func (t *TxDeleteObject) Clone() *TxDeleteObject {
	if t == nil {
		return nil
	}
	return &TxDeleteObject{
		ObjectKey:      t.GetObjectKey(),
		FailIfNotFound: t.GetFailIfNotFound(),
	}
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxDeleteObject) Validate() error {
	if len(t.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	return nil
}

// ExecuteTx executes the transaction against a world instance.
func (t *TxDeleteObject) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	lookupWorldOp world.LookupOp,
	worldInstance world.WorldState,
) (sysErr bool, rerr error) {
	if err := t.Validate(); err != nil {
		return false, err
	}

	objKey := t.GetObjectKey()

	// check if it exists, if necessary
	failNotFound := t.GetFailIfNotFound()
	if failNotFound {
		_, err := world.MustGetObject(ctx, worldInstance, objKey)
		if err != nil {
			return false, err
		}
	}

	// delete the object
	deleted, err := worldInstance.DeleteObject(ctx, t.GetObjectKey())
	if err == nil && failNotFound && !deleted {
		err = world.ErrObjectNotFound
	}
	return false, err
}

// _ is a type assertion
var _ Transaction = ((*TxDeleteObject)(nil))
