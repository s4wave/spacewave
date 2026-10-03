package repofs

import (
	"context"
	"sync"

	git_unixfs "github.com/s4wave/spacewave/db/git/unixfs"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/world"
)

// OpenRepoFSCursor opens a repo filesystem cursor for a git/repo object.
func OpenRepoFSCursor(
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	write bool,
) (unixfs.FSCursor, error) {
	// Acquire the repository object state for the cursor lifetime.
	objState, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		world.ReleaseObjectState(objState)
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}

	// Open the repository transaction with the requested write capability.
	eng := NewEngine(ctx, ws, objState)
	tx, err := eng.NewTransaction(ctx, write)
	if err != nil {
		eng.Close()
		world.ReleaseObjectState(objState)
		return nil, err
	}

	// Configure the Git cursor to observe changes and permit requested writes.
	opts := []git_unixfs.DotGitFSCursorOption{
		git_unixfs.WithDotGitChangeSource(eng),
	}
	if write {
		opts = append(opts, git_unixfs.WithDotGitWritable(true))
	}
	cursor := git_unixfs.NewDotGitFSCursorWithOptions(tx, "", opts...)
	return newRepoFSCursor(cursor, func() {
		tx.Discard()
		eng.Close()
		world.ReleaseObjectState(objState)
	}), nil
}

type repoFSCursor struct {
	cursor    unixfs.FSCursor
	releaseFn func()

	once sync.Once
}

func newRepoFSCursor(cursor unixfs.FSCursor, releaseFn func()) *repoFSCursor {
	return &repoFSCursor{
		cursor:    cursor,
		releaseFn: releaseFn,
	}
}

func (c *repoFSCursor) CheckReleased() bool {
	return c.cursor.CheckReleased()
}

func (c *repoFSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	return c.cursor.GetProxyCursor(ctx)
}

func (c *repoFSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	if cb == nil {
		return
	}
	c.cursor.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
		// Release the repository resources when the underlying cursor is released.
		if ch != nil && ch.Released {
			c.releaseOwned()
		}

		// Forward an absent change without replacing its cursor.
		if ch == nil {
			return cb(ch)
		}

		// Present the repository cursor to the change callback.
		next := ch.Clone()
		next.Cursor = c
		return cb(next)
	})
}

func (c *repoFSCursor) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	return c.cursor.GetCursorOps(ctx)
}

func (c *repoFSCursor) Release() {
	c.cursor.Release()
	c.releaseOwned()
}

func (c *repoFSCursor) releaseOwned() {
	c.once.Do(func() {
		if c.releaseFn != nil {
			c.releaseFn()
		}
	})
}

var _ unixfs.FSCursor = (*repoFSCursor)(nil)
