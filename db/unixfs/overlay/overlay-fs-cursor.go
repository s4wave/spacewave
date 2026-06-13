package unixfs_overlay

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// OverlayFSCursor composes a read-only lower cursor with a writable upper cursor.
type OverlayFSCursor struct {
	released atomic.Bool
	state    *overlayFSState
	parent   *OverlayFSCursor
	name     string
	lower    unixfs.FSCursor
	upper    unixfs.FSCursor
}

type overlayFSState struct {
	mtx sync.Mutex
}

// NewOverlayFSCursor constructs an overlay cursor from lower and upper root cursors.
func NewOverlayFSCursor(lower, upper unixfs.FSCursor) *OverlayFSCursor {
	return &OverlayFSCursor{
		state: &overlayFSState{},
		lower: lower,
		upper: upper,
	}
}

func newOverlayFSCursor(
	state *overlayFSState,
	parent *OverlayFSCursor,
	name string,
	lower unixfs.FSCursor,
	upper unixfs.FSCursor,
) *OverlayFSCursor {
	return &OverlayFSCursor{
		state:  state,
		parent: parent,
		name:   name,
		lower:  lower,
		upper:  upper,
	}
}

// CheckReleased checks if the fs cursor is currently released.
func (c *OverlayFSCursor) CheckReleased() bool {
	if c == nil || c.released.Load() {
		return true
	}
	if c.lower != nil && c.lower.CheckReleased() {
		return true
	}
	return c.upper != nil && c.upper.CheckReleased()
}

// GetProxyCursor returns nil, nil because overlay cursors do not redirect.
func (c *OverlayFSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	if c.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	return nil, nil
}

// AddChangeCb adds a change callback to detect when the cursor has changed.
func (c *OverlayFSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	if c.CheckReleased() {
		cb(&unixfs.FSCursorChange{Cursor: c, Released: true})
		return
	}
	wrap := func(ch *unixfs.FSCursorChange) bool {
		return cb(&unixfs.FSCursorChange{
			Cursor:   c,
			Released: ch.Released,
			Offset:   ch.Offset,
			Size:     ch.Size,
		})
	}
	if c.lower != nil {
		c.lower.AddChangeCb(wrap)
	}
	if c.upper != nil {
		c.upper.AddChangeCb(wrap)
	}
}

// GetCursorOps returns the FSCursorOps for the overlay cursor.
func (c *OverlayFSCursor) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	if c.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	c.state.mtx.Lock()
	defer c.state.mtx.Unlock()

	upperOps, upperErr := cursorOps(ctx, c.upper)
	if upperErr != nil && !isNotExist(upperErr) {
		return nil, upperErr
	}
	lowerOps, lowerErr := cursorOps(ctx, c.lower)
	if lowerErr != nil && !isNotExist(lowerErr) {
		return nil, lowerErr
	}

	if upperOps != nil {
		if lowerOps != nil && !sameNodeType(upperOps, lowerOps) {
			lowerOps = nil
		}
		return &OverlayFSCursorOps{
			c:        c,
			upperOps: upperOps,
			lowerOps: lowerOps,
			active:   upperOps,
		}, nil
	}
	if lowerOps != nil {
		if hidden, err := c.isHiddenByParentLocked(ctx); err != nil {
			return nil, err
		} else if hidden {
			return nil, unixfs_errors.ErrNotExist
		}
		return &OverlayFSCursorOps{
			c:        c,
			lowerOps: lowerOps,
			active:   lowerOps,
		}, nil
	}
	return nil, unixfs_errors.ErrNotExist
}

// Release releases the filesystem cursor.
func (c *OverlayFSCursor) Release() {
	if c == nil {
		return
	}
	c.released.Store(true)
	if c.lower != nil {
		c.lower.Release()
	}
	if c.upper != nil {
		c.upper.Release()
	}
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*OverlayFSCursor)(nil))
