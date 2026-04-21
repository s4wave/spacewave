package forge_lib_kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
	forge_target "github.com/s4wave/spacewave/forge/target"
)

// ApplyOpDelete applies a DELETE operation against a store.
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

	err := btx.Delete(ctx, key)
	if err != nil {
		return err
	}
	// set the output if necessary
	return nil
}
