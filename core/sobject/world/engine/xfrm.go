package sobject_world_engine

import (
	"crypto/rand"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/util/scrub"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/zeebo/blake3"
)

// BuildInitialInnerState builds the initialized empty world state.
func BuildInitialInnerState(initOp *InitWorldOp) (*InnerState, error) {
	var transformConf *block_transform.Config
	if initOp != nil {
		transformConf = initOp.GetTransformConf()
	}
	if transformConf.GetEmpty() {
		var err error
		transformConf, err = buildDefaultTransformConf()
		if err != nil {
			return nil, err
		}
	}

	return &InnerState{
		HeadRef: &bucket.ObjectRef{
			TransformConf: transformConf,
		},
	}, nil
}

// buildDefaultTransformConf builds the transform used to store the head state.
func buildDefaultTransformConf() (*block_transform.Config, error) {
	var encKey [32]byte
	var material [64]byte
	if _, err := rand.Read(material[:]); err != nil {
		return nil, err
	}
	defer scrub.Scrub(material[:])

	blake3.DeriveKey("sobject/world/engine transform-config Sat Oct 22 15:21:51 PDT 2024 v1.", material[:], encKey[:])

	return block_transform.NewConfig([]config.Config{&transform_blockenc.Config{
		BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
		Key:      encKey[:],
	}})
}
