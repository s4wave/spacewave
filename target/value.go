package forge_target

import (
	"bytes"
	"context"
	"io"

	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	block_msgpack "github.com/aperturerobotics/hydra/block/msgpack"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// AccessObject creates a block cursor at a object value.
func AccessObject(
	ctx context.Context,
	handle ExecControllerHandle,
	ref *bucket.ObjectRef,
	cb func(*block.Cursor) error,
) (*bucket.ObjectRef, error) {
	return world.AccessObject(ctx, handle.AccessStorage, ref, cb)
}

// AccessValue accesses object located at value and returns a new value with the
// updated result, or a clone of the original if nothing changed.
//
// Value can be nil (or the ref empty) to create a new value.
func AccessValue(
	ctx context.Context,
	handle ExecControllerHandle,
	val *forge_value.Value,
	cb func(*block.Cursor) error,
) (*forge_value.Value, error) {
	nval := val.Clone()
	if nval == nil {
		nval = &forge_value.Value{}
	}
	bref, err := nval.ToBucketRef()
	if err != nil {
		return nil, err
	}
	nobj, err := AccessObject(ctx, handle, bref, cb)
	if err != nil {
		return nil, err
	}
	if nobj.EqualsRef(bref) {
		return nval, nil
	}
	if nval.GetValueType() == forge_value.ValueType_ValueType_BLOCK_REF {
		return forge_value.NewValueWithBlockRef("", nobj.GetRootRef()), nil
	} else {
		return forge_value.NewValueWithBucketRef("", nobj), nil
	}
}

// StoreBlobValue stores the given data as a Blob and returns a BlockRef value.
func StoreBlobValue(
	ctx context.Context,
	handle ExecControllerHandle,
	dataLen int64,
	rd io.Reader,
) (*forge_value.Value, error) {
	return AccessValue(ctx, handle, nil, func(bcs *block.Cursor) error {
		_, err := blob.BuildBlob(
			ctx,
			dataLen,
			rd,
			bcs,
			nil,
		)
		return err
	})
}

// StoreBlobValueFromBytes stores the given []byte slice as a Blob value.
func StoreBlobValueFromBytes(
	ctx context.Context,
	handle ExecControllerHandle,
	data []byte,
) (*forge_value.Value, error) {
	return StoreBlobValue(ctx, handle, int64(len(data)), bytes.NewReader(data))
}

// LoadBlobValueToBytes loads the blob value to a byte slice.
func LoadBlobValueToBytes(
	ctx context.Context,
	handle ExecControllerHandle,
	val *forge_value.Value,
) ([]byte, error) {
	var out []byte
	_, err := AccessValue(ctx, handle, val, func(bcs *block.Cursor) error {
		var berr error
		out, berr = blob.FetchToBytes(ctx, bcs)
		return berr
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// StoreMsgpackValue stores the given data as a Msgpack block.
// The data is all stored in a single block.
func StoreMsgpackValue(
	ctx context.Context,
	handle ExecControllerHandle,
	value interface{},
) (*forge_value.Value, error) {
	return AccessValue(ctx, handle, nil, func(bcs *block.Cursor) error {
		err := block_msgpack.ObjectToBlock(bcs, value)
		return err
	})
}

// LoadMsgpackValue loads the data from a msgpack block.
// use interface{} type to unmarshal dynamic types.
// if ctor is nil, uses the empty value of T.
// returns the empty value returned from ctor if value is empty
// StoreMsgpackValue stores the given data as a Msgpack block.
func LoadMsgpackValue[T any](
	ctx context.Context,
	handle ExecControllerHandle,
	value *forge_value.Value,
	ctor func() T,
) (T, error) {
	if value.IsEmpty() {
		if ctor == nil {
			var empty T
			return empty, nil
		}
		return ctor(), nil
	}
	var outObj T
	_, err := AccessValue(ctx, handle, value, func(bcs *block.Cursor) error {
		outBlk, berr := block_msgpack.UnmarshalMsgpackBlock(bcs, ctor)
		if berr != nil {
			return berr
		}
		outObj = outBlk.GetObj()
		return nil
	})
	if err != nil {
		var empty T
		return empty, err
	}
	return outObj, nil
}

// StoreValueAsBlockRef copies the value to a BlockRef. Copies data into the
// state bucket if it is located in a different bucket.
//
// May return nil if there is no value set or if it is empty.
// Otherwise returns a *Value with type BLOCK_REF or an error.
func StoreValueAsBlockRef(
	ctx context.Context,
	handle ExecControllerHandle,
	val *forge_value.Value,
) (*forge_value.Value, error) {
	if val.IsEmpty() {
		return nil, nil
	}

	var err error
	vtype := val.GetValueType()
	if vtype == forge_value.ValueType_ValueType_BLOCK_REF {
		return forge_value.NewValueWithBlockRef("", val.GetBlockRef()), nil
	}

	var outValue *forge_value.Value
	err = handle.AccessStorage(
		ctx,
		nil,
		func(bls *bucket_lookup.Cursor) error {
			var berr error
			outValue, berr = CopyValueToBucket(ctx, handle, val)
			return berr
		},
	)
	return outValue, err
}

// CopyValueToBucket copies the value to the target bucket.
//
// May return nil if there is no value set or if it is empty.
// Otherwise returns a *Value with type BLOCK_REF or an error.
func CopyValueToBucket(
	ctx context.Context,
	handle ExecControllerHandle,
	val *forge_value.Value,
) (*forge_value.Value, error) {
	bktRef, err := val.ToBucketRef()
	if err != nil {
		return nil, err
	}

	outputRef := bktRef.GetRootRef()
	if bktRef.GetEmpty() || outputRef.GetEmpty() {
		return nil, nil
	}
	// fetch the data if the bucket id is different
	if bktRef.GetBucketId() != "" {
		var rootBlockData []byte
		var rootBlockFound bool
		err = handle.AccessStorage(
			ctx,
			bktRef,
			func(bls *bucket_lookup.Cursor) error {
				_, bcs := bls.BuildTransactionAtRef(nil, outputRef)
				// TODO: copy full reference graph
				// for now, just copy the root block.
				var berr error
				rootBlockData, rootBlockFound, berr = bcs.Fetch()
				return berr
			},
		)
		if err == nil && !rootBlockFound {
			err = errors.Errorf(
				"block %s in bucket %s not found",
				outputRef.MarshalString(),
				bktRef.GetBucketId(),
			)
		}
		if err != nil {
			return nil, err
		}
		err = handle.AccessStorage(ctx, nil, func(bls *bucket_lookup.Cursor) error {
			var berr error
			outputRef, _, berr = bls.PutBlock(rootBlockData, nil)
			return berr
		})
		if err != nil {
			return nil, err
		}
	}
	return forge_value.NewValueWithBlockRef("", outputRef), nil
}
