package world_block_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// NewTxApplyObjectOp constructs a new APPLY_OBJECT_OP transaction.
func NewTxApplyObjectOp(operationTypeID string, op world.Operation, objKey string) (*Tx, error) {
	opBody, err := op.MarshalBlock()
	if err != nil {
		return nil, err
	}
	return &Tx{
		TxType: TxType_TxType_APPLY_OBJECT_OP,
		TxApplyObjectOp: &TxApplyObjectOp{
			OperationTypeId: operationTypeID,
			OperationBody:   opBody,
			ObjectKey:       objKey,
		},
	}, nil
}

// NewTxApplyObjectOpTxn constructs a new APPLY_OBJECT_OP transaction.
func NewTxApplyObjectOpTxn() Transaction {
	return &TxApplyObjectOp{}
}

// GetTxType returns the type of transaction this is.
func (t *TxApplyObjectOp) GetTxType() TxType {
	return TxType_TxType_APPLY_OBJECT_OP
}

// Clone clones the tx object.
func (t *TxApplyObjectOp) Clone() *TxApplyObjectOp {
	if t == nil {
		return nil
	}
	body := make([]byte, len(t.GetOperationBody()))
	copy(body, t.GetOperationBody())
	return &TxApplyObjectOp{
		OperationTypeId: t.GetOperationTypeId(),
		OperationBody:   body,
		ObjectKey:       t.GetObjectKey(),
	}
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxApplyObjectOp) Validate() error {
	if len(t.GetOperationTypeId()) == 0 {
		return world.ErrEmptyOp
	}
	if len(t.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	return nil
}

// ExecuteTx executes the transaction against a world instance.
func (t *TxApplyObjectOp) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	lookupWorldOp world.LookupOp,
	lookupObjectOp world.LookupOp,
	worldInstance world.WorldState,
) (rerr error) {
	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(error); ok {
				rerr = v
			} else {
				rerr = errors.New("unmarshal operation paniced")
			}
		}
	}()

	if err := t.Validate(); err != nil {
		return err
	}

	// resolve + construct the operation type
	opTypeID := t.GetOperationTypeId()
	op, err := lookupObjectOp(opTypeID)
	if err == nil && op == nil {
		err = errors.Wrap(world.ErrUnhandledOp, opTypeID)
	}
	if err != nil {
		return err
	}

	// unmarshal the block
	err = op.UnmarshalBlock(t.GetOperationBody())
	if err != nil {
		return err
	}

	// lookup the object
	obj, err := world.MustGetObject(worldInstance, t.GetObjectKey())
	if err != nil {
		return err
	}

	// apply the operation
	_, err = obj.ApplyObjectOp(opTypeID, op, sender)
	return err
}

// _ is a type assertion
var _ Transaction = ((*TxApplyObjectOp)(nil))
