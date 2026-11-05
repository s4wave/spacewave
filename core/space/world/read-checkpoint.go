package space_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// OpenReadCheckpoint serves the World of snap, the state this participant held
// when its read access ended, without a live adapter. It replays the operation
// set as the Space World engine engineID would, keeping the replayed pages in
// memory and the overflow under a stage, and serves the World read-only.
// Release closes the engine and the stage.
func OpenReadCheckpoint(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	so sobject.SharedObject,
	engineID string,
	opTypes OpTypes,
	snap sobject.SharedObjectStateSnapshot,
) (world.Engine, func(), error) {
	// Replay the held operations onto the checkpoint's World in memory.
	sfs := transform_all.BuildFactorySet()
	a, releaseStage, err := replayOnce(ctx, le, b, sfs, so, engineID, opTypes, snap, defaultMaxPendingBytes)
	if err != nil {
		return nil, nil, err
	}
	state, _ := a.replay.head()
	if state.GetHeadRef().GetEmpty() {
		releaseStage()
		return nil, nil, errors.New("read checkpoint holds no World")
	}

	// Serve it read-only over the kept pages, holding the stage until the
	// engine closes.
	cursor, err := a.newCursor(ctx, state.GetHeadRef())
	if err != nil {
		releaseStage()
		return nil, nil, err
	}
	engine, err := world_block.NewEngine(ctx, le, cursor, a.lookupOp, nil, false)
	if err != nil {
		cursor.Release()
		releaseStage()
		return nil, nil, err
	}
	release := func() {
		_ = engine.Close()
		cursor.Release()
		releaseStage()
	}
	return &readCheckpointEngine{Engine: engine, bus: b, le: le}, release, nil
}

// readCheckpointEngine rejects mutations through every exported World surface.
type readCheckpointEngine struct {
	// Engine serves the replayed World.
	*world_block.Engine
	// bus resolves the read-only cursors.
	bus bus.Bus
	// le is the logger.
	le *logrus.Entry
}

// NewTransaction opens a read snapshot; a write request cannot acquire authority.
func (e *readCheckpointEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	if write {
		return nil, tx.ErrNotWrite
	}
	return e.Engine.NewTransaction(ctx, false)
}

// Sync has no writes or live head to publish.
func (e *readCheckpointEngine) Sync(context.Context) (bool, error) {
	return false, nil
}

// BuildStorageCursor returns a cursor whose bucket cannot accept raw writes.
func (e *readCheckpointEngine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	cursor, err := e.Engine.BuildStorageCursor(ctx)
	if err != nil {
		return nil, err
	}
	return e.readOnlyCursor(ctx, cursor, cursor.Release), nil
}

// StageWorldState rejects writes; a read checkpoint cannot stage them.
func (e *readCheckpointEngine) StageWorldState(context.Context) (world.WorldStage, error) {
	return nil, tx.ErrNotWrite
}

// AccessWorldState exposes a bounded read-only cursor at the requested root.
func (e *readCheckpointEngine) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	return e.Engine.AccessWorldState(ctx, ref, func(cursor *bucket_lookup.Cursor) error {
		return cb(e.readOnlyCursor(ctx, cursor, nil))
	})
}

// readOnlyCursor rebuilds cursor over a read-only view of its bucket. release
// runs when the returned cursor is released.
func (e *readCheckpointEngine) readOnlyCursor(ctx context.Context, cursor *bucket_lookup.Cursor, release func()) *bucket_lookup.Cursor {
	// Drop the volume so reads resolve through the bucket.
	opArgs := cursor.GetOpArgs()
	opArgs.VolumeId = ""

	// Wrap the bucket in a read-only store at the cursor's position.
	read := bucket_lookup.NewCursorWithRelease(
		ctx,
		e.bus,
		e.le,
		cursor.GetStepFactorySet(),
		block.NewReadOnlyStore(cursor.GetBucket()),
		cursor.GetTransformer(),
		cursor.GetRef(),
		opArgs,
		cursor.GetTransformConf(),
		release,
	)
	read.SetBucketIDOverride(cursor.GetBucketIDOverride())
	return read
}

// _ is a type assertion
var _ world.Engine = (*readCheckpointEngine)(nil)
