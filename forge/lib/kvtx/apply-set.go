package forge_lib_kvtx

import (
	"bytes"
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// ApplyOpSet applies a SET operation against a store.
// sets a reference to the location.
func ApplyOpSet(
	ctx context.Context,
	handle forge_target.ExecControllerHandle,
	btx kvtx.BlockTx,
	key []byte,
	value *forge_value.Value,
	valueIsBlob bool,
	outputName string,
) error {
	if bytes.Equal(key, []byte("test-1")) {
		_ = ctx.Err()
	}
	btxCursor := btx.GetCursor()
	blockStore, _ := btxCursor.GetBlockStore()
	if blockStore == nil {
		return errors.New("block transaction had no block store attached")
	}

	// copy the value into the same bucket as the tree if necessary
	// note: value will be nil if the input ref is empty
	var err error
	value, err = forge_target.CopyValueToBucket(ctx, handle, value)
	if err != nil {
		return err
	}

	// create a cursor at the location
	var nvalCursor *block.Cursor
	if !value.IsEmpty() {
		nvalCursor = btxCursor.Detach(false)
		nvalCursor.ClearAllRefs()
		nvalCursor.SetRefAtCursor(value.GetBlockRef(), true)
	}

	err = btx.SetCursorAtKey(ctx, key, nvalCursor, valueIsBlob)
	if err != nil {
		return err
	}

	// set the output if necessary
	if len(outputName) != 0 {
		outVal := value.Clone()
		outVal.Name = outputName
		setVals := forge_value.ValueSlice{outVal}
		err = handle.SetOutputs(ctx, setVals, false)
		if err != nil {
			return err
		}
	}

	return nil
}
