package forge_lib_kvtx

import (
	"context"

	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/pkg/errors"
)

// PendingOp contains a pending operation with arguments.
type PendingOp struct {
	// opType is the operation type
	opType OpType
	// key is the resolved key to apply to
	key []byte
	// value is the resolved value to assign at the key
	value *forge_value.Value
	// valueIsBlob indicates the value is a blob.
	valueIsBlob bool
	// outputName is the output to assign the result to
	outputName string
}

// NewPendingOp constructs a new pending op.
// value or outputName can be empty depending on the operation.
func NewPendingOp(
	opType OpType,
	key []byte,
	value *forge_value.Value,
	valueIsBlob bool,
	outputName string,
) *PendingOp {
	return &PendingOp{
		opType:      opType,
		key:         key,
		value:       value,
		valueIsBlob: valueIsBlob,
		outputName:  outputName,
	}
}

// Apply applies the operation to the store.
func (o *PendingOp) Apply(
	ctx context.Context,
	handle forge_target.ExecControllerHandle,
	btx kvtx.BlockTx,
) error {
	opType := o.opType
	if opType == OpType_OpType_NONE {
		return nil
	}

	isBlob := o.valueIsBlob
	switch opType {
	case OpType_OpType_SET_BLOB:
		isBlob = true
		fallthrough
	case OpType_OpType_SET:
		return ApplyOpSet(
			ctx,
			handle,
			btx,
			o.key,
			o.value, isBlob,
			o.outputName,
		)
	case OpType_OpType_DELETE:
		return ApplyOpDelete(ctx, handle, btx, o.key, o.outputName)
	case OpType_OpType_GET:
		return ApplyOpGet(ctx, handle, btx, o.key, o.outputName)
	case OpType_OpType_GET_EXISTS:
		return ApplyOpGetExists(ctx, handle, btx, o.key, o.outputName)
	case OpType_OpType_CHECK_BLOB:
		isBlob = true
		fallthrough
	case OpType_OpType_CHECK:
		return ApplyOpCheck(ctx, handle, btx, o.key, o.value, isBlob, o.outputName)
	case OpType_OpType_CHECK_EXISTS:
		return ApplyOpCheckExists(ctx, handle, btx, o.key, true)
	case OpType_OpType_CHECK_NOT_EXISTS:
		return ApplyOpCheckExists(ctx, handle, btx, o.key, false)
	default:
		return errors.Wrap(ErrUnknownOpType, opType.String())
	}
}
