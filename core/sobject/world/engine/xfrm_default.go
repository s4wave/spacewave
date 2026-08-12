package sobject_world_engine

import (
	"crypto/rand"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/util/scrub"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
)

// buildDefaultTransformConf builds the transform used to store the head state.
func buildDefaultTransformConf() (*block_transform.Config, error) {
	var encKey [32]byte
	var material [64]byte
	if _, err := rand.Read(material[:]); err != nil {
		return nil, err
	}
	defer scrub.Scrub(material[:])

	blockenc.DeriveKeySHA256("sobject/world/engine transform-config 2026-06-08 sha256 v1.", material[:], encKey[:])

	return block_transform.NewConfig([]config.Config{&transform_blockenc.Config{
		BlockEnc: blockenc.DefaultBlockEnc,
		Key:      encKey[:],
	}})
}
