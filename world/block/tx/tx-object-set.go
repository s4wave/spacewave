package world_block_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
)

// NewTxObjectSet constructs a new OBJECT_SET transaction.
func NewTxObjectSet(objKey string, rootRef *bucket.ObjectRef) (*Tx, error) {
	return &Tx{
		TxType: TxType_TxType_OBJECT_SET,
		TxObjectSet: &TxObjectSet{
			ObjectKey: objKey,
			RootRef:   rootRef,
		},
	}, nil
}

// NewTxObjectSetTxn constructs a new OBJECT_SET transaction.
func NewTxObjectSetTxn() Transaction {
	return &TxObjectSet{}
}

// GetTxType returns the type of transaction this is.
func (t *TxObjectSet) GetTxType() TxType {
	return TxType_TxType_OBJECT_SET
}

// GetEmpty checks if the tx is empty.
func (t *TxObjectSet) GetEmpty() bool {
	return t.GetObjectKey() == ""
}

// Clone clones the tx object.
func (t *TxObjectSet) Clone() *TxObjectSet {
	if t == nil {
		return nil
	}
	return &TxObjectSet{
		ObjectKey: t.GetObjectKey(),
		RootRef:   t.GetRootRef().Clone(),
	}
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxObjectSet) Validate() error {
	if len(t.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := t.GetRootRef().Validate(); err != nil {
		return err
	}
	return nil
}

// ExecuteTx executes the transaction against a world instance.
func (t *TxObjectSet) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	lookupWorldOp world.LookupOp,
	worldInstance world.WorldState,
) (sysErr bool, rerr error) {
	if err := t.Validate(); err != nil {
		return false, err
	}

	// get the object
	obj, err := world.MustGetObject(worldInstance, t.GetObjectKey())
	if err != nil {
		return false, err
	}

	// set the root ref
	_, err = obj.SetRootRef(t.GetRootRef())
	return false, err
}

// _ is a type assertion
var _ Transaction = ((*TxObjectSet)(nil))
