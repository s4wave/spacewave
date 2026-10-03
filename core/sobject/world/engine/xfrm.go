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

// NewInitWorldOp returns a copy of base, or an empty operation, with a fresh
// World transform when base names none. The author chooses the transform, so
// every member replays the same World.
func NewInitWorldOp(base *InitWorldOp) (*InitWorldOp, error) {
	op := base.CloneVT()
	if op == nil {
		op = &InitWorldOp{}
	}
	if op.GetTransformConf().GetEmpty() {
		conf, err := buildDefaultTransformConf()
		if err != nil {
			return nil, err
		}
		op.TransformConf = conf
	}
	return op, nil
}

// BuildInitialInnerState builds the empty World initOp initializes. initOp
// must name an authenticated World transform.
func BuildInitialInnerState(initOp *InitWorldOp) (*InnerState, error) {
	transformConf := initOp.GetTransformConf()
	if transformConf.GetEmpty() {
		return nil, errors.New("world init operation names no transform")
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
