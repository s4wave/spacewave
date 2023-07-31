package unixfs

import (
	"context"
)

// FSHandleCursor implements a FSCursor attached to a FSHandle.
type FSHandleCursor struct {
	// handle is the FSHandle
	handle *FSHandle
}

// NewFSHandleCursor constructs a new FSHandleCursor attached to the given FSHandle.
func NewFSHandleCursor(handle *FSHandle) *FSHandleCursor {
	return &FSHandleCursor{handle: handle}
}

// CheckReleased checks if the fscursor is released without locking anything.
func (f *FSHandleCursor) CheckReleased() bool {
	return f.handle.CheckReleased()
}

// GetFSCursorOps returns the interface implementing FSHandleCursorOps.
// Called after AddChangeCb and only if GetProxyCursor returns nil, nil.
// Return nil, nil to indicate this position is null (nothing here).
// Return nil, ErrReleased to indicate this FSHandleCursor was released.
func (f *FSHandleCursor) GetFSCursorOps(ctx context.Context) (FSCursorOps, error) {
	_, ops, err := f.handle.GetOps(ctx)
	if err != nil {
		return nil, err
	}
	return ops, nil
}

// Release releases the filesystem cursor.
// note: locks rmtx. must NOT be locked when calling
func (f *FSHandleCursor) Release() {
	f.handle.Release()
}

// AddChangeCb is not applicable.
func (f *FSHandleCursor) AddChangeCb(cb FSCursorChangeCb) {}

// GetProxyCursor is not applicable to a FSHandle cursor.
func (f *FSHandleCursor) GetProxyCursor(ctx context.Context) (FSCursor, error) {
	return nil, nil
}

// _ is a type assertion
var _ FSCursor = ((*FSHandleCursor)(nil))
