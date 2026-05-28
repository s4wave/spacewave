package sobject_world_engine

import (
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
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
