package world_block

import (
	"context"

	block_mock "github.com/aperturerobotics/hydra/block/mock"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
	world_mock "github.com/aperturerobotics/hydra/world/mock"
	"github.com/pkg/errors"
)

// BuildMockWorldState builds a mock world state.
func BuildMockWorldState(ctx context.Context, ocs *bucket_lookup.Cursor) (*WorldState, error) {
	return BuildWorldStateFromCursor(
		ctx,
		ocs,
		world.NewAccessWorldStateFunc(ocs),
		world_mock.GetMockWorldOpHandlers(),
		world_mock.GetMockObjectOpHandlers(),
	)
}

// BuildMockObject builds a mock object in a world.
func BuildMockObject(ctx context.Context, ws world.WorldState, objKey string) (world.ObjectState, error) {
	// construct a basic example object
	if objKey == "" {
		objKey = "test-obj-1"
	}
	var oref *bucket.ObjectRef
	err := ws.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		oref = bls.GetRef() // note: clones the ref
		obtx, obcs := bls.BuildTransactionAtRef(nil, nil)
		exb := &block_mock.Example{Msg: "Hello from " + objKey}
		obcs.SetBlock(exb, true)
		var err error
		oref.RootRef, obcs, err = obtx.Write(true)
		return err
	})
	if err != nil {
		return nil, err
	}

	// create the object in the world
	_, err = ws.CreateObject(objKey, oref)
	if err != nil {
		return nil, err
	}

	// lookup the object
	objState, found, err := ws.GetObject(objKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("expected to find object after create")
	}
	return objState, nil
}
