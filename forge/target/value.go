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

// CopyValueToBucket copies the block graph under the value into the storage
// the handle writes to, and returns it as a BLOCK_REF value there. A value
// already in that storage returns its root without a copy.
//
// The copy walks the source graph from the root and stops at each block the
// storage already holds whole (a completion proof, held under the handle's
// stage), so it reads and writes only the blocks the storage lacks, and a
// repeat copy reads only the root's proof. Blocks move as stored, without
// decoding, so the source and the storage must share a transform config.
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
	root := bktRef.GetRootRef()
	if bktRef.GetEmpty() || root.GetEmpty() {
		return nil, nil
	}

	// Copy the graph from another bucket under the handle's stage.
	if bktRef.GetBucketId() != "" {
		err = handle.AccessStorage(ctx, nil, func(dst *bucket_lookup.Cursor) error {
			return copyGraphToCursor(ctx, dst, bktRef)
		})
		if err != nil {
			return nil, err
		}
	}
	return forge_value.NewValueWithBlockRef("", root), nil
}

// copyGraphToCursor copies the block graph under ref into the bucket dst
// writes to, skipping each subtree that bucket holds whole.
func copyGraphToCursor(ctx context.Context, dst *bucket_lookup.Cursor, ref *bucket.ObjectRef) error {
	// Follow the reference into the source bucket.
	src, err := dst.FollowRef(ctx, ref)
	if err != nil {
		return err
	}
	defer src.Release()

	// Refuse a copy whose stored bytes the destination would decode wrongly.
	if !src.SameTransform(dst) {
		return errors.Errorf(
			"copy %s from bucket %s: source transform config differs from the destination",
			ref.GetRootRef().MarshalString(),
			ref.GetBucketId(),
		)
	}

	// Copy under the destination cursor's stage, forgetting finished blocks so
	// the copy holds only the path from the root to its frontier.
	c := block.NewStageCopy(dst.GetBucket(), dst.GetBlockStore(), false)
	err = c.Copy(ctx, src.GetBlockStore(), ref.GetRootRef(), block.GraphCopyOptions{Forget: true})
	if err != nil {
		return errors.Wrapf(err, "copy from bucket %s", ref.GetBucketId())
	}
	return c.Flush(ctx)
}
