//go:build !goscript

package testbed

import (
	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_chksum "github.com/s4wave/spacewave/db/block/transform/chksum"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto/blake3"
)

func newEngineTransformConfig(engineBucketID string) (*block_transform.Config, error) {
	key := make([]byte, 32)
	blake3.DeriveKey("hydra/world/testbed "+engineBucketID, []byte("testbed"), key)

	return block_transform.NewConfig([]config.Config{
		&transform_chksum.Config{},
		&transform_gzip.Config{},
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      key,
		},
	})
}
