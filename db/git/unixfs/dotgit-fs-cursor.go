package unixfs_git

import (
	"context"
	"sync"
	"sync/atomic"

	hydra_git "github.com/s4wave/spacewave/db/git"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// DotGitFSCursor implements unixfs.FSCursor for a materialized .git directory.
type DotGitFSCursor struct {
	isReleased   atomic.Bool
	tx           hydra_git.Tx
	node         *dotGitNode
	writable     bool
	changeSource DotGitFSCursorChangeSource
	writeState   *dotGitWriteState

	mtx             sync.Mutex
	cbs             unixfs.FSCursorChangeCbSlice
	changeSourceRel func()
	releaseFn       func()
}

// NewDotGitFSCursor creates a new read-only .git directory cursor.
func NewDotGitFSCursor(tx hydra_git.Tx, name string) *DotGitFSCursor {
	return NewDotGitFSCursorWithOptions(tx, name)
}

// NewDotGitFSCursorWithOptions creates a new .git directory cursor.
func NewDotGitFSCursorWithOptions(tx hydra_git.Tx, name string, opts ...DotGitFSCursorOption) *DotGitFSCursor {
	// Create the .git root and apply its cursor options.
	root := newDotGitRootNode()
	root.name = name
	conf := dotGitFSCursorOptions{}
	for _, opt := range opts {
		opt(&conf)
	}

	// Respect the Git transaction read-only mode.
	if tx != nil && tx.GetReadOnly() {
		conf.writable = false
	}

	// Limit the .git cursor release callback to one invocation.
	releaseFn := conf.releaseFn
	if releaseFn != nil {
		var once atomic.Bool
		baseReleaseFn := releaseFn
		releaseFn = func() {
			if once.CompareAndSwap(false, true) {
				baseReleaseFn()
			}
		}
	}

	// Construct the .git cursor with shared write state when writable.
	c := &DotGitFSCursor{
		tx:           tx,
		node:         root,
		writable:     conf.writable,
		changeSource: conf.changeSource,
		releaseFn:    releaseFn,
	}
	if c.writable {
		c.writeState = newDotGitWriteState()
	}

	// Subscribe the .git cursor to repository changes.
	c.attachChangeSource()
	return c
}

func newDotGitFSCursorFromNode(
	tx hydra_git.Tx,
	node *dotGitNode,
	writable bool,
	changeSource DotGitFSCursorChangeSource,
	writeState *dotGitWriteState,
) *DotGitFSCursor {
	c := &DotGitFSCursor{
		tx:           tx,
		node:         node,
		writable:     writable,
		changeSource: changeSource,
		writeState:   writeState,
	}
	c.attachChangeSource()
	return c
}

func (c *DotGitFSCursor) attachChangeSource() {
	if c == nil || c.changeSource == nil {
		return
	}
	c.changeSourceRel = c.changeSource.AddDotGitChangeCb(func() {
		c.Release()
	})
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
	// Ignore an absent .git change callback.
	if cb == nil {
		return
	}

	// Register the .git change callback while the cursor is live.
	var added bool
	c.mtx.Lock()
	if !c.CheckReleased() {
		c.cbs = append(c.cbs, cb)
		added = true
	}
	c.mtx.Unlock()

	// Notify the callback immediately when the .git cursor is already released.
	if !added {
		_ = cb(&unixfs.FSCursorChange{Cursor: c, Released: true})
	}
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
	// Prepare the .git cursor callbacks for release outside the lock.
	var (
		cbs             unixfs.FSCursorChangeCbSlice
		changeSourceRel func()
		releaseFn       func()
	)

	// Mark the .git cursor released under its mutex.
	c.mtx.Lock()
	if c.isReleased.Swap(true) {
		c.mtx.Unlock()
		return
	}

	// Detach the .git cursor callbacks and release functions.
	cbs = c.cbs
	c.cbs = nil
	changeSourceRel = c.changeSourceRel
	c.changeSourceRel = nil
	releaseFn = c.releaseFn
	c.releaseFn = nil
	c.mtx.Unlock()

	// Release the .git change subscription and transaction resource.
	if changeSourceRel != nil {
		changeSourceRel()
	}
	if releaseFn != nil {
		releaseFn()
	}

	// Notify .git cursor callbacks that the cursor was released.
	_ = cbs.CallCbs(&unixfs.FSCursorChange{Cursor: c, Released: true})
}

// _ is a type assertion
var _ unixfs.FSCursor = (*DotGitFSCursor)(nil)
