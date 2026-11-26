package repofs

import (
	"context"
	"sync"

	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/pkg/errors"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	hydra_git "github.com/s4wave/spacewave/db/git"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/world"
)

// projectionTx publishes a Git store root through its world object.
type projectionTx struct {
	// Store implements the Git storage operations.
	*git_block.Store

	// obj receives the committed repository root.
	obj world.ObjectState
	// stage owns a write transaction's blocks until Discard, after Commit
	// publishes the root that references them. Nil for a read transaction.
	stage world.WorldStage
	// rootCursor retains the storage root for the transaction lifetime.
	rootCursor *bucket_lookup.Cursor
	// locCursor retains the repository location for the transaction lifetime.
	locCursor *bucket_lookup.Cursor

	// once releases the store, its cursors and its stage on the first
	// Discard call.
	once sync.Once
}

// newProjectionTx opens a Git transaction for the current World object root.
func newProjectionTx(ctx context.Context, ws world.WorldState, obj world.ObjectState, write bool) (*projectionTx, error) {
	// Resolve the current repository snapshot.
	objRef, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get root ref")
	}

	// Stage a write transaction's blocks until Commit publishes its root.
	tx := &projectionTx{obj: obj}
	storage := world.WorldStorage(ws)
	if write {
		tx.stage, err = ws.StageWorldState(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "stage world state")
		}
		storage = tx.stage
	}

	// Retain the storage cursors until the transaction is discarded.
	tx.rootCursor, err = storage.BuildStorageCursor(ctx)
	if err != nil {
		tx.release()
		return nil, errors.Wrap(err, "build storage cursor")
	}
	tx.locCursor, err = tx.rootCursor.FollowRef(ctx, objRef)
	if err != nil {
		tx.release()
		return nil, errors.Wrap(err, "follow ref")
	}

	// Expose a writable transaction only when requested.
	btx, bcs := tx.locCursor.BuildTransaction(nil)
	if !write {
		btx = nil
	}

	// Validate the repository before exposing its Git store.
	repob, err := git_block.UnmarshalRepo(ctx, bcs)
	if err != nil {
		tx.release()
		return nil, errors.Wrap(err, "unmarshal repo")
	}
	if err := repob.Validate(); err != nil {
		tx.release()
		return nil, errors.Wrap(err, "validate repo")
	}

	// Open the Git store over the validated repository.
	tx.Store, err = git_block.NewStore(ctx, btx, bcs, &memory.IndexStorage{}, nil)
	if err != nil {
		tx.release()
		return nil, errors.Wrap(err, "create git store")
	}
	return tx, nil
}

// Commit writes the store before publishing its new object root.
func (t *projectionTx) Commit(ctx context.Context) error {
	// Finalize Git storage before making its root visible to readers.
	if err := t.Store.Commit(); err != nil {
		return err
	}
	if t.GetReadOnly() {
		return nil
	}

	// Publishing the object root wakes repository revision subscribers.
	nextRef := t.locCursor.GetRef()
	nextRef.RootRef = t.Store.GetRef().Clone()
	_, err := t.obj.SetRootRef(ctx, nextRef)
	return err
}

// Discard releases the store, both cursors and the stage exactly once.
func (t *projectionTx) Discard() {
	t.once.Do(func() {
		// Store.Close only releases cached packs and cancels its context.
		// It always returns nil.
		_ = t.Close()
		t.release()
	})
}

// release releases the cursors and stage the transaction holds.
func (t *projectionTx) release() {
	if t.locCursor != nil {
		t.locCursor.Release()
	}
	if t.rootCursor != nil {
		t.rootCursor.Release()
	}
	if t.stage != nil {
		t.stage.Release()
	}
}

// _ is a type assertion
var _ hydra_git.Tx = (*projectionTx)(nil)
