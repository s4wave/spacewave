package world_block_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// NewTxApplyWorldOp constructs a new APPLY_WORLD_OP transaction.
func NewTxApplyWorldOp(op world.Operation) (*Tx, error) {
	opTypeID := op.GetOperationTypeId()
	if opTypeID == "" {
		return nil, world.ErrEmptyOp
	}
	opBody, err := op.MarshalBlock()
	if err != nil {
		return nil, err
	}
	return &Tx{
		TxType: TxType_TxType_APPLY_WORLD_OP,
		TxApplyWorldOp: &TxApplyWorldOp{
			OperationTypeId: opTypeID,
			OperationBody:   opBody,
		},
	}, nil
}

// NewTxApplyWorldOpTxn constructs a new APPLY_WORLD_OP transaction.
func NewTxApplyWorldOpTxn() Transaction {
	return &TxApplyWorldOp{}
}

// IsNil returns if the object is nil.
func (t *TxApplyWorldOp) IsNil() bool {
	return t == nil
}

// GetTxType returns the type of transaction this is.
func (t *TxApplyWorldOp) GetTxType() TxType {
	return TxType_TxType_APPLY_WORLD_OP
}

// GetEmpty checks if the tx is empty.
func (t *TxApplyWorldOp) GetEmpty() bool {
	return t.GetOperationTypeId() == "" || len(t.GetOperationBody()) == 0
}

// Clone clones the tx object.
func (t *TxApplyWorldOp) Clone() *TxApplyWorldOp {
	if t == nil {
		return nil
	}
	body := make([]byte, len(t.GetOperationBody()))
	copy(body, t.GetOperationBody())
	return &TxApplyWorldOp{
		OperationTypeId: t.GetOperationTypeId(),
		OperationBody:   body,
	}
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxApplyWorldOp) Validate() error {
	if len(t.GetOperationTypeId()) == 0 {
		return world.ErrEmptyOp
	}
	return nil
}

// ExecuteTx executes the transaction against a world instance.
func (t *TxApplyWorldOp) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	lookupOp world.LookupOp,
	worldInstance world.WorldState,
) (sysErr bool, rerr error) {
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
		return false, err
	}

	// resolve + construct the operation type
	opTypeID := t.GetOperationTypeId()
	op, err := lookupOp(ctx, opTypeID)
	if err == nil && op == nil {
		err = errors.Wrap(world.ErrUnhandledOp, opTypeID)
	}
	if err != nil {
		return false, err
	}

	// unmarshal the block
	err = op.UnmarshalBlock(t.GetOperationBody())
	if err != nil {
		return false, err
	}

	// apply the operation
	_, sysErr, err = worldInstance.ApplyWorldOp(op, sender)
	return sysErr, err
}

// _ is a type assertion
var _ Transaction = ((*TxApplyWorldOp)(nil))
