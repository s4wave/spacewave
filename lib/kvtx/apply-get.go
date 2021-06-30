package forge_kvtx

import (
	"context"

	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/kvtx"
)

// ApplyOpGet applies a GET operation against a store.
// bls must be located in same bucket as btx.
func ApplyOpGet(
	ctx context.Context,
	handle forge_target.ExecControllerHandle,
	btx kvtx.BlockTx,
	key []byte,
	outputName string,
) error {
	bcs, err := btx.GetCursorAtKey(key)
	if err != nil {
		return err
	}
	// set the output if necessary
	if len(outputName) != 0 {
		outVal := forge_value.NewValueWithBlockRef(bcs.GetRef())
		outVal.Name = outputName
		setVals := forge_value.ValueSlice{outVal}
		err = handle.SetOutputs(ctx, setVals, false)
		if err != nil {
			return err
		}
	}
	return nil
}
