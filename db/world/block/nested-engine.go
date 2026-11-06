package world_block

import (
	"context"
	"math"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// NestedEngine is a read-only Engine over the World that a NestedWorld object
// publishes in an outer Engine. Its seqno is the outer object's revision, so
// its waits follow that object's key watch. Each read transaction retains the
// nested World at the revision it opened until Discard.
type NestedEngine struct {
	// le logs nested World state construction.
	le *logrus.Entry
	// outer holds the NestedWorld object this Engine follows.
	outer world.Engine
	// storage opens the nested World blocks under the granting scope.
	storage world.WorldStorage
	// lookupOp resolves World operations for the nested states.
	lookupOp world.LookupOp
	// key is the outer object that publishes the nested World.
	key string
}

// NewNestedEngine follows the NestedWorld object at key in outer. storage
// opens the nested World blocks, which live in the outer World's bucket.
func NewNestedEngine(
	le *logrus.Entry,
	outer world.Engine,
	storage world.WorldStorage,
	lookupOp world.LookupOp,
	key string,
) *NestedEngine {
	return &NestedEngine{le: le, outer: outer, storage: storage, lookupOp: lookupOp, key: key}
}

// OperationAuthor returns no author: the nested World accepts no writes.
func (e *NestedEngine) OperationAuthor(context.Context) (peer.ID, string, error) {
	return "", "", nil
}

// NewTransaction opens a read transaction on the nested World at the outer
// object's current revision. The transaction is read-only even when write is
// requested.
func (e *NestedEngine) NewTransaction(ctx context.Context, _ bool) (world.Tx, error) {
	// Read the revision and nested World root together.
	rev, worldRef, err := e.head(ctx)
	if err != nil {
		return nil, err
	}

	// Build the nested state, which pins its root until Discard.
	var state *WorldState
	err = e.storage.AccessWorldState(ctx, worldRef, func(cursor *bucket_lookup.Cursor) error {
		var buildErr error
		state, buildErr = BuildWorldStateFromCursor(ctx, e.le, false, cursor, e.storage, e.lookupOp, false)
		return buildErr
	})
	if err != nil {
		return nil, err
	}
	return &nestedTx{Tx: NewTx(state), rev: rev}, nil
}

// Sync does nothing: the nested World accepts no writes.
func (e *NestedEngine) Sync(context.Context) (bool, error) {
	return false, nil
}

// SyncThrough has nothing to fence: the nested World accepts no writes. It
// returns once the outer object reaches revision seqno.
func (e *NestedEngine) SyncThrough(ctx context.Context, seqno uint64) (uint64, error) {
	return e.WaitSeqno(ctx, seqno)
}

// BuildStorageCursor builds a cursor to the outer World storage.
func (e *NestedEngine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return e.storage.BuildStorageCursor(ctx)
}

// AccessWorldState builds a cursor at ref, or at the current nested World root
// when ref is empty. The cursor is released after cb returns.
func (e *NestedEngine) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	if ref.GetEmpty() {
		var err error
		if _, ref, err = e.head(ctx); err != nil {
			return err
		}
	}
	return e.storage.AccessWorldState(ctx, ref, cb)
}

// StageWorldState returns tx.ErrNotWrite: the nested World accepts no writes.
func (e *NestedEngine) StageWorldState(context.Context) (world.WorldStage, error) {
	return nil, tx.ErrNotWrite
}

// GetSeqno returns the outer object's current revision.
func (e *NestedEngine) GetSeqno(ctx context.Context) (uint64, error) {
	var rev uint64
	err := world.ExecTransaction(ctx, e.outer, false, func(ctx context.Context, ws world.WorldState) error {
		// Read the revision, treating a missing object as an error.
		var found bool
		var err error
		rev, found, err = world.GetObjectRev(ctx, ws, e.key)
		if err == nil && !found {
			err = world.ErrObjectNotFound
		}
		return err
	})
	return rev, err
}

// WaitSeqno waits for the outer object to reach revision value.
func (e *NestedEngine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	return e.outer.WaitObjectRev(ctx, e.key, value, false)
}

// WaitObjectRev waits for a nested object revision by rereading it after each
// outer object revision.
func (e *NestedEngine) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	return world.WaitObjectRevBySeqno(ctx, e, key, rev, ignoreNotFound)
}

// WatchChanges reports each outer object revision after afterSeqno as an
// unknown change: the nested World keeps no record of what changed between
// two of its roots, so every watcher relists.
func (e *NestedEngine) WatchChanges(ctx context.Context, afterSeqno uint64, _ world.ChangeFilter, cb func(uint64, *world.ChangeSet) error) error {
	return world.WatchSeqnoChanges(ctx, e, afterSeqno, cb)
}

// head reads the outer object's revision and nested World root from one outer
// read transaction.
func (e *NestedEngine) head(ctx context.Context) (uint64, *bucket.ObjectRef, error) {
	var rev uint64
	var worldRef *bucket.ObjectRef
	err := world.ExecTransaction(ctx, e.outer, false, func(ctx context.Context, ws world.WorldState) error {
		// Read the revision the nested root belongs to.
		var found bool
		var err error
		rev, found, err = world.GetObjectRev(ctx, ws, e.key)
		if err != nil {
			return err
		}
		if !found {
			return world.ErrObjectNotFound
		}

		// Read the nested World root from the same revision.
		nested, err := world.LookupObjectBody[NestedWorld](ctx, ws, e.key)
		if err != nil {
			return err
		}
		worldRef = nested.GetWorldRef()
		if worldRef.GetRootRef() == nil {
			return errors.Errorf("object %s has no nested World root", e.key)
		}
		return nil
	})
	return rev, worldRef, err
}

// nestedTx is a nested World read transaction whose seqno is the outer object
// revision it opened at.
type nestedTx struct {
	world.Tx
	// rev is the outer object revision the transaction retains.
	rev uint64
}

// GetSeqno returns the outer object revision the transaction retains.
func (t *nestedTx) GetSeqno(context.Context) (uint64, error) {
	return t.rev, nil
}

// WaitSeqno returns the retained revision once it satisfies value. A read
// snapshot never advances, so a later value waits until Discard or
// cancellation.
func (t *nestedTx) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	if value <= t.rev {
		return t.rev, nil
	}
	_, err := t.Tx.WaitSeqno(ctx, math.MaxUint64)
	return 0, err
}

// _ is a type assertion
var (
	_ world.Engine = (*NestedEngine)(nil)
	_ world.Tx     = (*nestedTx)(nil)
)
