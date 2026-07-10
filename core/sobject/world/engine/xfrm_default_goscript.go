//go:build goscript

package sobject_world_engine

import (
	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
)

// buildDefaultTransformConf keeps GoScript off BLAKE3-backed block encryption.
func buildDefaultTransformConf() (*block_transform.Config, error) {
	return block_transform.NewConfig([]config.Config{
		&transform_gzip.Config{},
	})
}
