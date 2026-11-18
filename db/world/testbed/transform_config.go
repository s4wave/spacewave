//go:build !goscript

package testbed

import (
	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/zeebo/blake3"
)

// NewEngineTransformConfig returns the testbed block transform: gzip, then
// encryption under a key derived from the engine bucket ID, so each testbed
// World stores its blocks as production does.
func NewEngineTransformConfig(engineBucketID string) (*block_transform.Config, error) {
	key := make([]byte, 32)
	blake3.DeriveKey("hydra/world/testbed "+engineBucketID, []byte("testbed"), key)

	return block_transform.NewConfig([]config.Config{
		&transform_gzip.Config{},
		&transform_blockenc.Config{
			BlockEnc: blockenc.DefaultBlockEnc,
			Key:      key,
		},
	})
}
