package bldr_dist_compiler

import (
	"bytes"

	"github.com/aperturerobotics/controllerbus/config"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/util/blockenc"
)

// buildEmbedTransformConf is the block transform conf to use for the embedded manifest world.
func buildEmbedTransformConf(workingID string) []config.Config {
	var key [32]byte
	material := bytes.Join([][]byte{[]byte("embed manifest blockenc"), []byte(workingID)}, []byte("--- SENTIENT CLOUD ---"))
	blockenc.DeriveKeySHA256("bldr dist compiler embed transform conf 2026-06-08 sha256 v1", material, key[:])
	return []config.Config{
		&transform_s2.Config{},
		&transform_blockenc.Config{
			BlockEnc: blockenc.DefaultBlockEnc,
			Key:      key[:],
		},
	}
}
