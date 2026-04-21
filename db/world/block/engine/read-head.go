package world_block_engine

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/object"
	"github.com/sirupsen/logrus"
)

// ReadHeadRef reads the HEAD reference from the given object store.
// This is the same data that the engine controller persists via commitFn.
// stateTransformConf may be nil if no transform is used.
func ReadHeadRef(
	ctx context.Context,
	le *logrus.Entry,
	store object.ObjectStore,
	headKey string,
	stateTransformConf *block_transform.Config,
	sfs *block_transform.StepFactorySet,
) (*bucket.ObjectRef, error) {
	if headKey == "" {
		headKey = defaultHeadStateKey
	}

	ktx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer ktx.Discard()

	data, found, err := ktx.Get(ctx, []byte(headKey))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	if !stateTransformConf.GetEmpty() {
		xfrm, err := block_transform.NewTransformer(
			controller.ConstructOpts{Logger: le},
			sfs,
			stateTransformConf,
		)
		if err != nil {
			return nil, err
		}
		data, err = xfrm.DecodeBlock(data)
		if err != nil {
			return nil, err
		}
	}

	s := &HeadState{}
	if err := s.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return s.GetHeadRef(), nil
}
