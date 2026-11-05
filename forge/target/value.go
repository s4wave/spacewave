package forge_target

import (
	"bytes"
	"context"
	"io"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	forge_value "github.com/s4wave/spacewave/forge/value"
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
	// Resolve a cloned value to the bucket reference used for object access.
	nval := val.Clone()
	if nval == nil {
		nval = &forge_value.Value{}
	}
	bref, err := nval.ToBucketRef()
	if err != nil {
		return nil, err
	}

	// Access the value object and retain the cloned value if its reference is unchanged.
	nobj, err := AccessObject(ctx, handle, bref, cb)
	if err != nil {
		return nil, err
	}
	if nobj.EqualsRef(bref) {
		return nval, nil
	}

	// Represent the updated object using the original value reference type.
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

	vtype := val.GetValueType()
	if vtype == forge_value.ValueType_ValueType_BLOCK_REF {
		return forge_value.NewValueWithBlockRef("", val.GetBlockRef()), nil
	}

	return CopyValueToBucket(ctx, handle, val)
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
	// Resolve the source value to its bucket reference.
	bktRef, err := val.ToBucketRef()
	if err != nil {
		return nil, err
	}

	// Return no value when the source bucket or root reference is empty.
	outputRef := bktRef.GetRootRef()
	if bktRef.GetEmpty() || outputRef.GetEmpty() {
		return nil, nil
	}

	// Copy the root block from another bucket into the target storage, with
	// the child list the target Volume requires of every block it stores.
	// Only the root moves; the blocks it names are not copied.
	if bktRef.GetBucketId() != "" {
		// Read the source root's bytes and child list, and decode the bytes.
		var root *block.StoredBlock
		err = handle.AccessStorage(
			ctx,
			bktRef,
			func(bls *bucket_lookup.Cursor) error {
				// Read the stored root and require its child list.
				var berr error
				root, berr = bls.GetBlockStore().GetStoredBlock(ctx, outputRef)
				if berr != nil || root == nil {
					return berr
				}
				if !root.RefsKnown {
					return errors.Wrap(block.ErrRefsUnknown, outputRef.MarshalString())
				}

				// Decode the bytes with the source bucket's transformer.
				if xfrm := bls.GetTransformer(); xfrm != nil {
					root.Data, berr = xfrm.DecodeBlock(root.Data)
				}
				return berr
			},
		)
		if err == nil && root == nil {
			err = errors.Errorf(
				"block %s in bucket %s not found",
				outputRef.MarshalString(),
				bktRef.GetBucketId(),
			)
		}
		if err != nil {
			return nil, err
		}

		// Persist the root block in the target bucket.
		err = handle.AccessStorage(ctx, nil, func(bls *bucket_lookup.Cursor) error {
			var berr error
			outputRef, _, berr = bls.PutBlock(ctx, root.Data, &block.PutOpts{
				Sync:      true,
				Refs:      root.Refs,
				RefsKnown: true,
			})
			return berr
		})
		if err != nil {
			return nil, err
		}
	}
	return forge_value.NewValueWithBlockRef("", outputRef), nil
}
