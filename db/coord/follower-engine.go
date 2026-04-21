//go:build !js

package coord

import (
	"context"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ReadHeadRefFunc reads the current HEAD reference for the world state.
// Called by the follower to refresh its view after a commit counter change.
type ReadHeadRefFunc func(ctx context.Context) (*bucket.ObjectRef, error)

// FollowerEngine provides a read-only world.Engine for follower processes.
// It watches the bbolt commit counter and calls SetRootRef on the underlying
// world_block.Engine when the leader commits changes.
type FollowerEngine struct {
	le       *logrus.Entry
	db       *bdb.DB
	engine   *world_block.Engine
	readHead ReadHeadRefFunc
	lastHead *bucket.ObjectRef
}

// NewFollowerEngine creates a follower engine. The baseCursor provides block
// storage access (same bbolt bucket/volume as the leader). readHead reads
// the current HEAD reference from the object store. lookupOp resolves
// operation types for world state construction.
func NewFollowerEngine(
	ctx context.Context,
	le *logrus.Entry,
	db *bdb.DB,
	baseCursor *bucket_lookup.Cursor,
	readHead ReadHeadRefFunc,
	lookupOp world.LookupOp,
) (*FollowerEngine, error) {
	headRef, err := readHead(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "read initial head ref")
	}

	cursor, err := baseCursor.FollowRef(ctx, headRef)
	if err != nil {
		return nil, errors.Wrap(err, "follow initial head ref")
	}

	engine, err := world_block.NewEngine(ctx, le, cursor, lookupOp, nil, false)
	if err != nil {
		cursor.Release()
		return nil, errors.Wrap(err, "create engine")
	}

	return &FollowerEngine{
		le:       le,
		db:       db,
		engine:   engine,
		readHead: readHead,
		lastHead: headRef,
	}, nil
}

// Run watches the commit counter and refreshes the engine root reference.
// Blocks until ctx is cancelled.
func (f *FollowerEngine) Run(ctx context.Context) error {
	var lastCounter uint64
	for {
		counter, err := f.db.WaitCommitCounter(ctx, lastCounter)
		if err != nil {
			return err
		}
		lastCounter = counter

		headRef, err := f.readHead(ctx)
		if err != nil {
			f.le.WithError(err).Warn("failed to read head ref")
			continue
		}

		// Skip update if HEAD hasn't changed.
		if f.lastHead != nil && headRef != nil && f.lastHead.EqualsRef(headRef) {
			continue
		}

		if err := f.engine.SetRootRef(ctx, headRef); err != nil {
			f.le.WithError(err).Warn("failed to update root ref")
			continue
		}
		f.lastHead = headRef
	}
}

// NewTransaction returns a read-only transaction. Write transactions
// are not supported on followers; use the leader's SubmitWorldOp SRPC.
func (f *FollowerEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	if write {
		return nil, errors.New("follower engine is read-only")
	}
	return f.engine.NewTransaction(ctx, false)
}

// GetSeqno returns the current world state sequence number.
func (f *FollowerEngine) GetSeqno(ctx context.Context) (uint64, error) {
	return f.engine.GetSeqno(ctx)
}

// WaitSeqno waits for the world state seqno to reach the given value.
func (f *FollowerEngine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	return f.engine.WaitSeqno(ctx, value)
}

// BuildStorageCursor builds a cursor to the world storage.
func (f *FollowerEngine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return f.engine.BuildStorageCursor(ctx)
}

// AccessWorldState builds a bucket lookup cursor for reading world state.
func (f *FollowerEngine) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return f.engine.AccessWorldState(ctx, ref, cb)
}

// _ is a type assertion.
var _ world.Engine = (*FollowerEngine)(nil)
