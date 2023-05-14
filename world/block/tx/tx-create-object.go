package world_block_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
)

// NewTxCreateObject constructs a new CREATE_OBJECT transaction.
func NewTxCreateObject(objKey string, rootRef *bucket.ObjectRef) (*Tx, error) {
	return &Tx{
		TxType: TxType_TxType_CREATE_OBJECT,
		TxCreateObject: &TxCreateObject{
			ObjectKey: objKey,
			RootRef:   rootRef,
		},
	}, nil
}

// NewTxCreateObjectTxn constructs a new CREATE_OBJECT transaction.
func NewTxCreateObjectTxn() Transaction {
	return &TxCreateObject{}
}

// IsNil returns if the object is nil.
func (t *TxCreateObject) IsNil() bool {
	return t == nil
}

// GetTxType returns the type of transaction this is.
func (t *TxCreateObject) GetTxType() TxType {
	return TxType_TxType_CREATE_OBJECT
}

// Clone clones the tx object.
func (t *TxCreateObject) Clone() *TxCreateObject {
	if t == nil {
		return nil
	}
	return &TxCreateObject{
		ObjectKey: t.GetObjectKey(),
		RootRef:   t.GetRootRef().Clone(),
	}
}

// GetEmpty checks if the tx is empty.
func (t *TxCreateObject) GetEmpty() bool {
	return t.GetObjectKey() == ""
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxCreateObject) Validate() error {
	if len(t.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := t.GetRootRef().Validate(); err != nil {
		return err
	}
	return nil
}

// ExecuteTx executes the transaction against a world instance.
func (t *TxCreateObject) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	lookupWorldOp world.LookupOp,
	worldInstance world.WorldState,
) (sysErr bool, rerr error) {
	if err := t.Validate(); err != nil {
		return false, err
	}

	// create the object
	_, err := worldInstance.CreateObject(ctx, t.GetObjectKey(), t.GetRootRef())
	return false, err
}

// _ is a type assertion
var _ Transaction = ((*TxCreateObject)(nil))
