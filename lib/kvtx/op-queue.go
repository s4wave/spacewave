package forge_kvtx

import (
	"context"

	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/pkg/errors"
)

// OpQueue prefetches input values for a set of ops.
type OpQueue struct {
	ctx       context.Context
	inputVals forge_value.ValueMap
	handle    forge_target.ExecControllerHandle
	pending   []*PendingOp
}

// NewOpQueue constructs a new op queue.
func NewOpQueue(
	ctx context.Context,
	inputVals forge_value.ValueMap,
	handle forge_target.ExecControllerHandle,
) *OpQueue {
	return &OpQueue{
		ctx:       ctx,
		inputVals: inputVals,
		handle:    handle,
	}
}

// GetPendingOps returns the slice of pending ops.
func (q *OpQueue) GetPendingOps() []*PendingOp {
	return q.pending
}

// ApplyOps applies all pending ops.
func (q *OpQueue) ApplyOps(btx kvtx.BlockTx, clear, ignoreErr bool) error {
	pending := q.pending
	if clear {
		q.pending = nil
	}
	for _, op := range pending {
		if err := op.Apply(q.ctx, q.handle, btx); err != nil {
			if !ignoreErr {
				return err
			}
		}
	}
	return nil
}

// AddOps adds a list of ops to the queue.
// Resolves the input values or returns an error.
func (q *OpQueue) AddOps(ops []*Op) error {
	for i, op := range ops {
		err := q.AddOp(op)
		if err != nil {
			if err == context.Canceled {
				return err
			}
			return errors.Wrapf(err, "ops[%d]", i)
		}
	}
	return nil
}

// AddOp applies an operation to the queue.
func (q *OpQueue) AddOp(op *Op) error {
	if err := op.Validate(); err != nil {
		return err
	}

	return q.addOpRecurse(op, 0, nil, nil, false, "")
}

// AddPendingOp queues a pending op.
func (q *OpQueue) AddPendingOp(op *PendingOp) int {
	if op != nil && len(op.key) != 0 {
		q.pending = append(q.pending, op)
	}
	return len(q.pending)
}

// addOpRecurse applies an operation with optional parent values.
// applies nested operations recursively
func (q *OpQueue) addOpRecurse(
	op *Op,
	prevOpType OpType,
	prevKey []byte,
	prevInputValue *forge_value.Value,
	prevInputValueBlob bool,
	prevOutput string,
) error {
	// determine operation key
	opKey, err := q.resolveKeyInput(op)
	if err != nil {
		return err
	}
	if len(opKey) == 0 {
		opKey = prevKey
	}

	// determine input value if set
	inputValue, inputValueBlob, err := q.resolveValueInput(op)
	if err != nil {
		return err
	}
	if inputValue == nil {
		inputValue = prevInputValue
		inputValueBlob = prevInputValueBlob
	}

	// determine output name
	outputName := op.GetOutput()
	if len(outputName) == 0 {
		outputName = prevOutput
	}

	// queue operation
	opType := op.GetOpType()
	if opType == OpType_OpType_NONE {
		opType = prevOpType
	}
	if opType != OpType_OpType_NONE && len(opKey) != 0 {
		_ = q.AddPendingOp(NewPendingOp(
			opType,
			opKey,
			inputValue,
			inputValueBlob,
			outputName,
		))
	}

	// recurse into sub-operations
	for i, op := range op.GetOps() {
		err = q.addOpRecurse(
			op,
			opType,
			opKey,
			inputValue, inputValueBlob,
			outputName,
		)
		if err != nil {
			if err == context.Canceled {
				return err
			}
			return errors.Wrapf(err, "ops[%d]", i)
		}
	}
	return nil
}

// resolveKeyInput resolves the key field, either in-line or via an Input.
// returns nil if the key was not configured.
func (q *OpQueue) resolveKeyInput(op *Op) ([]byte, error) {
	opKey := []byte(op.GetKey())
	if len(opKey) == 0 {
		keyInputName := op.GetKeyInput()
		if len(keyInputName) != 0 {
			inpVal, ok := q.inputVals[keyInputName]
			if !ok {
				return nil, nil
			}
			bktRef, err := inpVal.ToBucketRef()
			if err != nil {
				return nil, err
			}
			err = q.handle.AccessStorage(
				q.ctx, bktRef,
				func(bls *bucket_lookup.Cursor) error {
					_, bcs := bls.BuildTransactionAtRef(nil, bktRef.GetRootRef())
					var berr error
					opKey, _, berr = bcs.Fetch()
					return berr
				},
			)
			if err != nil {
				return nil, err
			}
		}
	}
	return opKey, nil
}

// resolveValueInput resolves the Value field, either in-line or via an Input.
// returns nil if the value was not configured.
func (q OpQueue) resolveValueInput(op *Op) (*forge_value.Value, bool, error) {
	var inputValueBlob bool
	inputValue := op.GetValue()
	if inputValue.IsEmpty() {
		inputValue = nil
	}
	var err error
	if valueStr := op.GetValueString(); len(valueStr) != 0 {
		// copy the inline value string to a Blob & store
		inputValue, err = forge_target.StoreBlobValueFromBytes(
			q.ctx,
			q.handle,
			[]byte(valueStr),
		)
		if err != nil {
			return nil, false, err
		}
		inputValueBlob = inputValue != nil
	}
	if valueInputName := op.GetValueInput(); valueInputName != "" {
		var ok bool
		inputValue, ok = q.inputVals[valueInputName]
		if !ok || inputValue.IsEmpty() {
			return nil, false, errors.Wrap(forge_value.ErrUnsetValue, valueInputName)
		}
	}
	if inputValue == nil {
		inputValue = op.GetValue()
		inputValueBlob = false
	}
	return inputValue, inputValueBlob, nil
}
