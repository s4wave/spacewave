package unixfs_empty

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// FSCursor implements a FSCursor which emulates an empty directory.
type FSCursor struct {
	isReleased atomic.Bool
}

// NewFSCursor creates a new FSCursor.
func NewFSCursor() *FSCursor {
	return &FSCursor{}
}

func (e *FSCursor) CheckReleased() bool {
	return e.isReleased.Load()
}

func (e *FSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	return nil, nil
}

func (e *FSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	// No-op: empty directory never changes
}

func (e *FSCursor) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	if e.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	return &FSCursorOps{cursor: e}, nil
}

func (e *FSCursor) Release() {
	e.isReleased.Store(true)
}

var _ unixfs.FSCursor = (*FSCursor)(nil)
