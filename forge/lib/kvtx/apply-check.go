package forge_lib_kvtx

import (
	"bytes"
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/kvtx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// ApplyOpCheck applies a CHECK operation against a store.
func ApplyOpCheck(
	ctx context.Context,
	handle forge_target.ExecControllerHandle,
	btx kvtx.BlockTx,
	key []byte,
	value *forge_value.Value,
	valueIsBlob bool,
	outputName string,
) error {
	// Open the stored value cursor for the checked key.
	bcs, err := btx.GetCursorAtKey(ctx, key)
	if err != nil {
		return err
	}

	// Publish the stored reference when the check requests an output.
	bcsRef := bcs.GetRef()
	outVal := forge_value.NewValueWithBlockRef("", bcsRef)
	if len(outputName) != 0 {
		outVal.Name = outputName
		setVals := forge_value.ValueSlice{outVal}
		err = handle.SetOutputs(ctx, setVals, false)
		if err != nil {
			return err
		}
	}

	// Compare blob contents when the input value represents a blob.
	if valueIsBlob {
		var wasEqual bool
		_, err = forge_target.AccessValue(ctx, handle, value, func(cs *block.Cursor) error {
			var berr error
			wasEqual, berr = blob.CompareBlobs(ctx, cs, bcs)
			return berr
		})
		if err == nil && !wasEqual {
			err = ErrValueMismatch
		}
		return err
	}

	// if in different buckets, need to compare values
	valueBktRef := value.GetBucketRef()
	if value.GetValueType() == forge_value.ValueType_ValueType_BUCKET_REF &&
		valueBktRef.GetBucketId() != "" {
		bcsData, bcsOk, err := bcs.Fetch(ctx)
		if err != nil {
			return err
		}
		if !bcsOk {
			return errors.New("referenced stored value was not found")
		}
		var wasEqual bool
		_, err = forge_target.AccessValue(ctx, handle, value, func(cs *block.Cursor) error {
			// Fetch the input block and require its contents to exist.
			data, dataOk, err := cs.Fetch(ctx)
			if err != nil {
				return err
			}
			if !dataOk {
				return errors.New("input value not found")
			}

			// Compare the input block contents with the stored block contents.
			wasEqual = bytes.Equal(data, bcsData)
			return nil
		})
		if err != nil {
			return err
		}
		if !wasEqual {
			return ErrValueMismatch
		}
		return nil
	}

	// compare references
	if !value.GetBlockRef().EqualsRef(bcsRef) {
		return ErrValueMismatch
	}
	return nil
}

// ApplyOpCheckExists ensures a key does or does not exist.
func ApplyOpCheckExists(
	ctx context.Context,
	handle forge_target.ExecControllerHandle,
	btx kvtx.BlockTx,
	key []byte,
	shouldExist bool,
) error {
	doesExist, err := btx.Exists(ctx, key)
	if err != nil {
		return err
	}
	if doesExist != shouldExist {
		if shouldExist {
			return errors.New("key does not exist")
		}
		return errors.New("key exists")
	}
	return nil
}
