//go:build goscript

package testbed

import (
	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_chksum "github.com/s4wave/spacewave/db/block/transform/chksum"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
)

func newEngineTransformConfig(string) (*block_transform.Config, error) {
	return block_transform.NewConfig([]config.Config{
		&transform_chksum.Config{},
		&transform_gzip.Config{},
	})
}
