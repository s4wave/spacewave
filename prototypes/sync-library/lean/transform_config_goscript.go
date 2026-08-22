//go:build goscript

package lean

import (
	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
)

// engineTransformConfig mirrors the world testbed goscript configuration.
func engineTransformConfig(string) (*block_transform.Config, error) {
	return block_transform.NewConfig([]config.Config{
		&transform_gzip.Config{},
	})
}
