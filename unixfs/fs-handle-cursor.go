package unixfs

import (
	"context"
	"sync/atomic"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// FSHandleCursor implements a FSCursor attached to a FSHandle.
type FSHandleCursor struct {
	// released indicates this FSHandleCursor was released
	released atomic.Bool
	// handle is the FSHandle
	handle *FSHandle
	// releaseHandle indicates we should release the FSHandle when released.
	releaseHandle bool
	// relFunc is the function to call when released
	// may be nil
	relFunc func()
}

// NewFSHandleCursor constructs a new FSHandleCursor attached to the given FSHandle.
//
// if releaseHandle is set, the Release function will also release the FSHandle.
// if relFunc is set, the release function will be called when the FSHandleCursor is released.
func NewFSHandleCursor(handle *FSHandle, releaseHandle bool, relFunc func()) *FSHandleCursor {
	return &FSHandleCursor{handle: handle, releaseHandle: releaseHandle, relFunc: relFunc}
}

// CheckReleased checks if the fscursor is released without locking anything.
func (f *FSHandleCursor) CheckReleased() bool {
	if f.handle.CheckReleased() {
		f.released.Store(true)
		return true
	}
	return f.released.Load()
}

// GetCursorOps returns the interface implementing FSHandleCursorOps.
// Called after AddChangeCb and only if GetProxyCursor returns nil, nil.
// Return nil, nil to indicate this position is null (nothing here).
// Return nil, ErrReleased to indicate this FSHandleCursor was released.
func (f *FSHandleCursor) GetCursorOps(ctx context.Context) (FSCursorOps, error) {
	if f.released.Load() {
		return nil, unixfs_errors.ErrReleased
	}
	if f.handle.CheckReleased() {
		f.released.Store(true)
		return nil, unixfs_errors.ErrReleased
	}
	_, ops, err := f.handle.GetOps(ctx)
	if err != nil {
		return nil, err
	}
	return ops, nil
}

// Release releases the filesystem cursor.
// note: locks rmtx. must NOT be locked when calling
func (f *FSHandleCursor) Release() {
	if !f.released.Swap(true) && f.releaseHandle {
		f.handle.Release()
		if f.relFunc != nil {
			f.relFunc()
		}
	}
}

// AddChangeCb is not applicable.
func (f *FSHandleCursor) AddChangeCb(cb FSCursorChangeCb) {}

// GetProxyCursor is not applicable to a FSHandle cursor.
func (f *FSHandleCursor) GetProxyCursor(ctx context.Context) (FSCursor, error) {
	return nil, nil
}

// _ is a type assertion
var _ FSCursor = ((*FSHandleCursor)(nil))
