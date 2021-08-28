package world_block_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// NewTxApplyWorldOp constructs a new APPLY_WORLD_OP transaction.
func NewTxApplyWorldOp(operationTypeID string, op world.Operation) (*Tx, error) {
	opBody, err := op.MarshalBlock()
	if err != nil {
		return nil, err
	}
	return &Tx{
		TxType: TxType_TxType_APPLY_WORLD_OP,
		TxApplyWorldOp: &TxApplyWorldOp{
			OperationTypeId: operationTypeID,
			OperationBody:   opBody,
		},
	}, nil
}

// NewTxApplyWorldOpTxn constructs a new APPLY_WORLD_OP transaction.
func NewTxApplyWorldOpTxn() Transaction {
	return &TxApplyWorldOp{}
}

// GetTxType returns the type of transaction this is.
func (t *TxApplyWorldOp) GetTxType() TxType {
	return TxType_TxType_APPLY_WORLD_OP
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
	op, err := lookupWorldOp(opTypeID)
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

	// apply the operation
	_, err = worldInstance.ApplyWorldOp(opTypeID, op, sender)
	return err
}

// _ is a type assertion
var _ Transaction = ((*TxApplyWorldOp)(nil))
