package forge_kvtx

import (
	"bytes"
	"context"
	"errors"

	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/kvtx"
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
	if bytes.Compare(key, []byte("test-1")) == 0 {
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
	value, err = forge_target.CopyValueToBucket(ctx, handle, value, blockStore)
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
	err = btx.SetCursorAtKey(key, nvalCursor, valueIsBlob)
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
