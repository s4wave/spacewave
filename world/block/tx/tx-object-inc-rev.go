package world_block_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/world"
)

// NewTxObjectIncRev constructs a new OBJECT_INC_REV transaction.
func NewTxObjectIncRev(objKey string) (*Tx, error) {
	return &Tx{
		TxType: TxType_TxType_OBJECT_INC_REV,
		TxObjectIncRev: &TxObjectIncRev{
			ObjectKey: objKey,
		},
	}, nil
}

// NewTxObjectIncRevTxn constructs a new OBJECT_INC_REV transaction.
func NewTxObjectIncRevTxn() Transaction {
	return &TxObjectIncRev{}
}

// GetTxType returns the type of transaction this is.
func (t *TxObjectIncRev) GetTxType() TxType {
	return TxType_TxType_OBJECT_INC_REV
}

// GetEmpty checks if the tx is empty.
func (t *TxObjectIncRev) GetEmpty() bool {
	return t.GetObjectKey() == ""
}

// Clone clones the tx object.
func (t *TxObjectIncRev) Clone() *TxObjectIncRev {
	if t == nil {
		return nil
	}
	return &TxObjectIncRev{
		ObjectKey: t.GetObjectKey(),
	}
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxObjectIncRev) Validate() error {
	if len(t.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	return nil
}

// ExecuteTx executes the transaction against a world instance.
func (t *TxObjectIncRev) ExecuteTx(
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

	// inc the revision
	_, err = obj.IncrementRev()
	return false, err
}

// _ is a type assertion
var _ Transaction = ((*TxObjectIncRev)(nil))
