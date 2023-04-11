package bldr_dist_compiler

import (
	"bytes"

	"github.com/aperturerobotics/controllerbus/config"
	transform_blockenc "github.com/aperturerobotics/hydra/block/transform/blockenc"
	transform_s2 "github.com/aperturerobotics/hydra/block/transform/s2"
	"github.com/aperturerobotics/hydra/util/blockenc"
	"github.com/zeebo/blake3"
)

// buildEmbedTransformConf is the block transform conf to use for the embedded manifest world.
func buildEmbedTransformConf(workingID string) []config.Config {
	var key [32]byte
	material := bytes.Join([][]byte{[]byte("embed manifest blockenc"), []byte(workingID)}, []byte("--- SENTIENT CLOUD ---"))
	blake3.DeriveKey("bldr dist compiler embed transform conf Tue Apr 11 02:54:00 PM PDT 2023", material, key[:])
	return []config.Config{
		&transform_s2.Config{Best: true},
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      key[:],
		},
	}
}
