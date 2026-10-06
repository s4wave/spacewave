package world_block_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// NewTxApplyObjectOp constructs a new APPLY_OBJECT_OP transaction.
func NewTxApplyObjectOp(operationTypeID string, op world.Operation, objKey string, opSender peer.ID) (*Tx, error) {
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
			OpSender:        opSender.String(),
		},
	}, nil
}

// NewTxApplyObjectOpTxn constructs a new APPLY_OBJECT_OP transaction.
func NewTxApplyObjectOpTxn() Transaction {
	return &TxApplyObjectOp{}
}

// IsNil returns if the object is nil.
func (t *TxApplyObjectOp) IsNil() bool {
	return t == nil
}

// GetTxType returns the type of transaction this is.
func (t *TxApplyObjectOp) GetTxType() TxType {
	return TxType_TxType_APPLY_OBJECT_OP
}

// GetEmpty checks if the tx is empty.
func (t *TxApplyObjectOp) GetEmpty() bool {
	return t.GetObjectKey() == "" || t.GetOperationTypeId() == ""
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
		OpSender:        t.GetOpSender(),
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
	if opSender := t.GetOpSender(); opSender != "" {
		if _, err := confparse.ParsePeerID(opSender); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteTx executes the transaction against a world instance.
func (t *TxApplyObjectOp) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	lookupWorldOp world.LookupOp,
	worldInstance world.WorldState,
) (sysErr bool, rerr error) {
	// Translate operation decoding panics into transaction errors.
	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(error); ok {
				rerr = v
			} else {
				rerr = errors.New("unmarshal operation paniced")
			}
		}
	}()

	// Decode the operation and resolve its recorded sender.
	op, err := t.decodeOp(ctx, lookupWorldOp)
	if err != nil {
		return false, err
	}
	if opSender := t.GetOpSender(); opSender != "" {
		sender, err = confparse.ParsePeerID(opSender)
		if err != nil {
			return false, err
		}
	}

	// Look up the object and apply the operation to it.
	obj, err := world.MustGetObject(ctx, worldInstance, t.GetObjectKey())
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return false, err
	}
	_, _, err = obj.ApplyObjectOp(ctx, op, sender)
	return false, nameMissingBlock(err, t.GetOperationTypeId()+" on "+t.GetObjectKey(), op)
}

// decodeOp validates the transaction and decodes its operation, resolving the
// operation type with lookupOp.
func (t *TxApplyObjectOp) decodeOp(ctx context.Context, lookupOp world.LookupOp) (world.Operation, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return decodeOperation(ctx, lookupOp, t.GetOperationTypeId(), t.GetOperationBody())
}

// _ is a type assertion
var _ Transaction = (*TxApplyObjectOp)(nil)
