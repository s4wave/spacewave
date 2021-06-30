package forge_kvtx

import (
	"context"
	"errors"

	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/kvtx"
)

// ApplyOpSet applies a SET operation against a store.
// sets a reference to the location.
// bls must be located in same bucket as btx.
func ApplyOpSet(
	ctx context.Context,
	handle forge_target.ExecControllerHandle,
	btx kvtx.BlockTx,
	key []byte,
	value *forge_value.Value,
	valueIsBlob bool,
	outputName string,
) error {
	btxCursor := btx.GetCursor()
	blockStore, _ := btxCursor.GetBlockStore()
	if blockStore == nil {
		return errors.New("block transaction had no block store attached")
	}

	// copy the value into the same bucket as the tree if necessary
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
		nvalCursor.SetRefAtCursor(value.GetBlockRef())
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
