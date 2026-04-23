package unixfs_git

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/go-git/go-git/v6/storage"
)

// DotGitFSCursor implements unixfs.FSCursor for a materialized .git directory.
type DotGitFSCursor struct {
	isReleased atomic.Bool
	storer     storage.Storer
	node       *dotGitNode
}

// NewDotGitFSCursor creates a new read-only .git directory cursor.
func NewDotGitFSCursor(storer storage.Storer, name string) *DotGitFSCursor {
	root := newDotGitRootNode()
	root.name = name
	return &DotGitFSCursor{
		storer: storer,
		node:   root,
	}
}

func newDotGitFSCursorFromNode(storer storage.Storer, node *dotGitNode) *DotGitFSCursor {
	return &DotGitFSCursor{
		storer: storer,
		node:   node,
	}
}

// CheckReleased checks if the cursor is released.
func (c *DotGitFSCursor) CheckReleased() bool {
	return c.isReleased.Load()
}

// GetProxyCursor returns nil, nil because repository layout nodes do not proxy.
func (c *DotGitFSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	if c.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	return nil, nil
}

// AddChangeCb is a no-op for the initial read-only cursor.
func (c *DotGitFSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	// noop
}

// GetCursorOps returns the FSCursorOps for this cursor.
func (c *DotGitFSCursor) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	if c.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	return newDotGitFSCursorOps(c), nil
}

// Release releases the cursor.
func (c *DotGitFSCursor) Release() {
	c.isReleased.Store(true)
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*DotGitFSCursor)(nil))
