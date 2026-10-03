package sobject_world_engine

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
	"github.com/sirupsen/logrus"
)

// OpenReadCheckpoint serves the World of snap, the state this participant held
// when its read access ended, without starting a live body controller. It
// replays the operation set as the World engine engineID would. Release closes
// the cursor after all readers have released their transactions.
func OpenReadCheckpoint(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	so sobject.SharedObject,
	engineID string,
	snap sobject.SharedObjectStateSnapshot,
) (world.Engine, func(), error) {
	// Replay the held operations onto the checkpoint's World.
	sfs := transform_all.BuildFactorySet()
	state, err := ReplayWorld(ctx, le, b, sfs, so, engineID, nil, snap)
	if err != nil {
		return nil, nil, err
	}
	head := state.GetHeadRef()
	if head == nil {
		return nil, nil, errors.New("read checkpoint holds no World")
	}

	// Open a read-only block engine on the replayed World.
	engine, err := buildBlockEngine(ctx, le, b, sfs, so, head, head.GetTransformConf(), nil, false)
	if err != nil {
		return nil, nil, err
	}
	return &readCheckpointEngine{Engine: engine.bengine, bus: b, le: le}, engine.Release, nil
}

// readCheckpointEngine rejects mutations through every exported World surface.
type readCheckpointEngine struct {
	world.Engine
	bus bus.Bus
	le  *logrus.Entry
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

// AccessWorldState exposes a bounded read-only cursor at the requested root.
func (e *readCheckpointEngine) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	return e.Engine.AccessWorldState(ctx, ref, func(cursor *bucket_lookup.Cursor) error {
		return cb(e.readOnlyCursor(ctx, cursor, nil))
	})
}

func (e *readCheckpointEngine) readOnlyCursor(ctx context.Context, cursor *bucket_lookup.Cursor, release func()) *bucket_lookup.Cursor {
	opArgs := cursor.GetOpArgs()
	opArgs.VolumeId = ""
	read := bucket_lookup.NewCursorWithRelease(
		ctx,
		e.bus,
		e.le,
		cursor.GetStepFactorySet(),
		&readOnlyBlockStore{StoreOps: cursor.GetBucket()},
		cursor.GetTransformer(),
		cursor.GetRef(),
		opArgs,
		cursor.GetTransformConf(),
		release,
	)
	read.SetBucketIDOverride(cursor.GetBucketIDOverride())
	return read
}

// readOnlyBlockStore preserves block reads while rejecting every storage mutation.
type readOnlyBlockStore struct {
	block.StoreOps
}

func (s *readOnlyBlockStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := s.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &readOnlyBlockStore{StoreOps: store}, release, nil
}

func (s *readOnlyBlockStore) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, tx.ErrNotWrite
}

func (s *readOnlyBlockStore) PutBlockBatch(context.Context, []*block.PutBatchEntry) error {
	return tx.ErrNotWrite
}

func (s *readOnlyBlockStore) RmBlock(context.Context, *block.BlockRef) error {
	return tx.ErrNotWrite
}

func (s *readOnlyBlockStore) Sync(context.Context) (bool, error) {
	return false, tx.ErrNotWrite
}
