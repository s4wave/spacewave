package forge_lib_kvtx

import (
	"context"

	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/kvtx"
)

// ApplyOpGet applies a GET operation against a store.
func ApplyOpGet(
	ctx context.Context,
	handle forge_target.ExecControllerHandle,
	btx kvtx.BlockTx,
	key []byte,
	outputName string,
) error {
	bcs, err := btx.GetCursorAtKey(ctx, key)
	if err != nil {
		return err
	}
	// set the output if necessary
	if len(outputName) != 0 {
		outVal := forge_value.NewValueWithBlockRef("", bcs.GetRef())
		outVal.Name = outputName
		setVals := forge_value.ValueSlice{outVal}
		err = handle.SetOutputs(ctx, setVals, false)
		if err != nil {
			return err
		}
	}
	return nil
}

// ApplyOpGetExists applies a EXISTS operation against a store.
func ApplyOpGetExists(
	ctx context.Context,
	handle forge_target.ExecControllerHandle,
	btx kvtx.BlockTx,
	key []byte,
	outputName string,
) error {
	doesExist, err := btx.Exists(ctx, key)
	if err != nil {
		return err
	}
	// set the output if necessary
	if len(outputName) != 0 {
		outVal, err := forge_target.StoreMsgpackValue(ctx, handle, doesExist)
		if err != nil {
			return err
		}
		outVal.Name = outputName
		setVals := forge_value.ValueSlice{outVal}
		err = handle.SetOutputs(ctx, setVals, false)
		if err != nil {
			return err
		}
	}
	return nil
}
