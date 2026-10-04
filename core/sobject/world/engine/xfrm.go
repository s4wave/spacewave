package sobject_world_engine

import (
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
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

// newWorldTransformer builds the World block transformer. conf must name an
// authenticated encryption step.
func newWorldTransformer(
	opts controller.ConstructOpts,
	sfs *block_transform.StepFactorySet,
	conf *block_transform.Config,
) (*block_transform.Transformer, error) {
	if err := validateWorldWriteTransform(conf); err != nil {
		return nil, err
	}
	return block_transform.NewTransformer(opts, sfs, conf)
}

// validateWorldWriteTransform returns errUnauthenticatedWorldTransform unless
// conf has a valid authenticated encryption step.
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
