package dist_entrypoint

import (
	"github.com/aperturerobotics/controllerbus/config"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
)

// buildStorageTransformConf is the block transform conf to use for the world storage.
func buildStorageTransformConf(_ string) []config.Config {
	return []config.Config{
		&transform_s2.Config{},
	}
}
