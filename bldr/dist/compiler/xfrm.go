package bldr_dist_compiler

import (
	"github.com/aperturerobotics/controllerbus/config"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
)

// buildEmbedTransformConf is the block transform conf to use for the embedded manifest world.
func buildEmbedTransformConf(_ string) []config.Config {
	return []config.Config{
		&transform_s2.Config{},
	}
}
