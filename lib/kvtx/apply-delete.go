package forge_kvtx

import (
	"context"

	forge_target "github.com/aperturerobotics/forge/target"
	"github.com/aperturerobotics/hydra/kvtx"
)

// ApplyOpDelete applies a DELETE operation against a store.
// bls must be located in same bucket as btx.
func ApplyOpDelete(
	ctx context.Context,
	handle forge_target.ExecControllerHandle,
	btx kvtx.BlockTx,
	key []byte,
	outputName string,
) error {
	if len(outputName) != 0 {
		// get previous value
		err := ApplyOpGet(ctx, handle, btx, key, outputName)
		if err != nil {
			return err
		}
	}

	err := btx.Delete(key)
	if err != nil {
		return err
	}
	// set the output if necessary
	return nil
}
