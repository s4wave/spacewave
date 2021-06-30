package forge_kvtx

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/kvtx"
)

// ApplyOpDelete applies a DELETE operation against a store.
// bls must be located in same bucket as btx.
func ApplyOpDelete(
	ctx context.Context,
	btx kvtx.BlockTx,
	key []byte,
	outputName string,
) error {
	err := btx.Delete(key)
	if err != nil {
		return err
	}
	if len(outputName) != 0 {
		return errors.New("TODO set output for DELETE op")
	}
	return nil
}
