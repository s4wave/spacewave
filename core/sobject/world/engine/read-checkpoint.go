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
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// worldHeadStoreID is the local state store holding the head of the World
// this participant last installed.
const worldHeadStoreID = "world-head"

// worldHeadKey is the key of the head in the world head store.
var worldHeadKey = []byte("head")

// writeWorldHead records head as the last installed World. It stays after this
// participant loses read access, so its read checkpoint can serve the World it
// last held.
func writeWorldHead(ctx context.Context, so sobject.SharedObject, head *bucket.ObjectRef) error {
	// Store the head without its local bucket.
	stored := head.CloneVT()
	stored.BucketId = ""
	data, err := stored.MarshalVT()
	if err != nil {
		return err
	}
	store, release, err := so.AccessLocalStateStore(ctx, worldHeadStoreID, nil)
	if err != nil {
		return err
	}
	defer release()
	return kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, worldHeadKey, data)
	})
}

// readWorldHead returns the head writeWorldHead last recorded, or nil.
func readWorldHead(ctx context.Context, so sobject.SharedObject) (*bucket.ObjectRef, error) {
	// Open the local state store.
	store, release, err := so.AccessLocalStateStore(ctx, worldHeadStoreID, nil)
	if err != nil {
		return nil, err
	}
	defer release()
	var head *bucket.ObjectRef
	err = kvtx.RunTransaction(ctx, false, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, false)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		// Read the head, if any.
		head = nil
		data, found, err := tx.Get(ctx, worldHeadKey)
		if err != nil || !found {
			return err
		}
		head = &bucket.ObjectRef{}
		return head.UnmarshalVT(data)
	})
	return head, err
}

// OpenReadCheckpoint serves the World this participant last installed without
// starting a live body controller. Release closes the cursor after all readers
// have released their transactions.
func OpenReadCheckpoint(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	so sobject.SharedObject,
) (world.Engine, func(), error) {
	// Open a block engine on the last installed World head.
	head, err := readWorldHead(ctx, so)
	if err != nil {
		return nil, nil, err
	}
	if head == nil {
		return nil, nil, errors.New("no World was retained for the read checkpoint")
	}
	engine, err := buildBlockEngine(ctx, le, b, transform_all.BuildFactorySet(), so, head, head.GetTransformConf(), nil, false)
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
