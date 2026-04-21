package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// BuildMockWorldState builds a mock world state.
func BuildMockWorldState(ctx context.Context, le *logrus.Entry, write bool, ocs *bucket_lookup.Cursor, verbose bool) (*WorldState, error) {
	return BuildWorldStateFromCursor(
		ctx,
		le,
		write,
		ocs,
		world.NewWorldStorageFromCursor(ocs),
		world_mock.LookupMockOp,
		verbose,
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
		oref.RootRef, _, err = obtx.Write(ctx, true)
		return err
	})
	if err != nil {
		return nil, err
	}

	// create the object in the world
	_, err = ws.CreateObject(ctx, objKey, oref)
	if err != nil {
		return nil, err
	}

	// lookup the object
	objState, found, err := ws.GetObject(ctx, objKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("expected to find object after create")
	}
	return objState, nil
}

// MockObject is the mock object block.
type MockObject = block_mock.Example

// NewMockObjectBlock constructs a new mock object block.
func NewMockObjectBlock() block.Block {
	return block_mock.NewExampleBlock()
}

// UnmarshalMockObject unmarshals a block from a block cursor.
func UnmarshalMockObject(ctx context.Context, bcs *block.Cursor) (*block_mock.Example, error) {
	return block.UnmarshalBlock[*block_mock.Example](ctx, bcs, block_mock.NewExampleBlock)
}
