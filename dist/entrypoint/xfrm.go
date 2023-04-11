package dist_entrypoint

import (
	"bytes"

	"github.com/aperturerobotics/controllerbus/config"
	transform_blockenc "github.com/aperturerobotics/hydra/block/transform/blockenc"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	transform_s2 "github.com/aperturerobotics/hydra/block/transform/s2"
	"github.com/aperturerobotics/hydra/util/blockenc"
	"github.com/zeebo/blake3"
)

var baseMagic = []byte{0x4, 0x2, 0x0}
var secondMagic = [8]byte{0x4c, 0x47, 0x4c, 0x48, 0x4d, 0x0, 0x4, 0x2}

func xor(data []byte) []byte {
	out := make([]byte, len(data))
	for i := range data {
		out[i] = data[i] ^ secondMagic[i%len(secondMagic)] ^ baseMagic[i%len(baseMagic)]
	}
	return out
}

func getBldrMagic() []byte {
	return xor(
		[]byte{0x9e, 0x3a, 0x3a, 0x41, 0x70, 0xc3, 0x26, 0xf7, 0x30, 0xad, 0x3d, 0xfa, 0x24, 0x1e, 0x56, 0x1, 0x90, 0xed, 0xc1, 0x21, 0x1f, 0x57, 0x71, 0xa8, 0xba, 0x4b, 0xee, 0x39, 0xd3, 0x9, 0xca, 0x29},
	)
}

// buildStorageTransformConf is the block transform conf to use for the world storage.
func buildStorageTransformConf(projectID string) []config.Config {
	var key [32]byte
	material := bytes.Join([][]byte{getBldrMagic(), xor([]byte(projectID))}, []byte("--- COMBUSTIBLE LEMON ---"))
	blake3.DeriveKey("bldr dist entrypoint Tue Apr 11 01:33:30 PM PDT 2023", material, key[:])
	return []config.Config{
		&transform_chksum.Config{},
		&transform_s2.Config{},
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      key[:],
		},
	}
}
