package unixfs_access

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/unixfs"
)

// FSCursor implements a FSCursor attached to a UnixFS Access function.
type FSCursor struct {
	// isReleased indicates if this cursor is released.
	isReleased atomic.Bool
	// accessFn is the access function
	accessFn AccessUnixFSFunc
}

// NewFSCursor constructs a new FSCursor attached to the UnixFS Access function.
func NewFSCursor(accessFn AccessUnixFSFunc) *FSCursor {
	return &FSCursor{accessFn: accessFn}
}

// CheckReleased checks if the fscursor is released without locking anything.
func (f *FSCursor) CheckReleased() bool {
	return f.isReleased.Load()
}

// GetProxyCursor returns a FSCursor to replace this one, if necessary.
func (f *FSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	var relCb atomic.Pointer[func()]
	relFn := func() {
		if fn := relCb.Load(); fn != nil {
			(*fn)()
		}
	}
	fsHandle, fsHandleRel, err := f.accessFn(ctx, relFn)
	if err != nil {
		return nil, err
	}
	fsHandle.AddReleaseCallback(fsHandleRel)
	return unixfs.NewFSHandleCursor(fsHandle), nil
}

// Release releases the filesystem cursor.
// note: locks rmtx. must NOT be locked when calling
func (f *FSCursor) Release() {
	f.isReleased.Store(true)
}

// AddChangeCb is not applicable (GetProxyCursor returns a value).
func (f *FSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {}

// GetFSCursorOps is not applicable (GetProxyCursor returns a value).
func (f *FSCursor) GetFSCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	return nil, nil
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*FSCursor)(nil))
