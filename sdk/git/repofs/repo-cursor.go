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
	objState, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}

	eng := NewEngine(ctx, ws, objState)
	tx, err := eng.NewTransaction(ctx, write)
	if err != nil {
		eng.Close()
		return nil, err
	}

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
		if ch != nil && ch.Released {
			c.releaseOwned()
		}
		if ch == nil {
			return cb(ch)
		}
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

// _ is a type assertion
var _ unixfs.FSCursor = ((*repoFSCursor)(nil))
