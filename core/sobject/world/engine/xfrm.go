package sobject_world_engine

import (
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/util/blockenc"
)

var errUnauthenticatedWorldTransform = errors.New("world transform requires authenticated encryption")

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
	if err := validateWorldWriteTransform(transformConf); err != nil {
		return nil, err
	}

	return &InnerState{
		HeadRef: &bucket.ObjectRef{
			TransformConf: transformConf,
		},
	}, nil
}

// worldTransformer permits legacy plaintext world reads but rejects every
// write until the Space has migrated to authenticated encryption.
type worldTransformer struct {
	*block_transform.Transformer
	writeErr error
}

func newWorldTransformer(
	opts controller.ConstructOpts,
	sfs *block_transform.StepFactorySet,
	conf *block_transform.Config,
) (*worldTransformer, error) {
	xfrm, err := block_transform.NewTransformer(opts, sfs, conf)
	if err != nil {
		return nil, err
	}
	return &worldTransformer{
		Transformer: xfrm,
		writeErr:    validateWorldWriteTransform(conf),
	}, nil
}

// EncodeBlock encodes an authenticated world block.
func (t *worldTransformer) EncodeBlock(data []byte) ([]byte, error) {
	if t.writeErr != nil {
		return nil, t.writeErr
	}
	return t.Transformer.EncodeBlock(data)
}

func validateWorldWriteTransform(conf *block_transform.Config) error {
	for _, step := range conf.GetSteps() {
		if step.GetId() != transform_blockenc.ConfigID {
			continue
		}
		encConf := &transform_blockenc.Config{}
		if err := block_transform.UnmarshalStepConfig(step.GetConfig(), encConf); err != nil {
			return errors.Wrap(err, "world block encryption config")
		}
		switch encConf.GetBlockEnc() {
		case blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			blockenc.BlockEnc_BlockEnc_SECRET_BOX,
			blockenc.BlockEnc_BlockEnc_AES_256_GCM:
			if err := encConf.Validate(); err != nil {
				return errors.Wrap(err, "world block encryption config")
			}
			return nil
		}
	}
	return errUnauthenticatedWorldTransform
}

var (
	_ block.Transformer                  = (*worldTransformer)(nil)
	_ block.DecodedBlockCacheTransformer = (*worldTransformer)(nil)
)
