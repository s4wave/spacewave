package unixfs

import (
	"context"
	"sync/atomic"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// FSCursorGetter implements a FSCursor with a getter function.
// The value from the getter is returned in GetProxyCursor.
// If the getter returns nil, nil, returns nil, ErrNotExist instead.
// If the getter function is nil, returns ErrNotExist.
// The context passed to the getter should not be used after the getter returns.
// CheckReleased never returns false until Release is called.
type FSCursorGetter struct {
	released atomic.Bool
	getter   func(ctx context.Context) (FSCursor, error)
}

// NewFSCursorGetter constructs a new FSCursorGetter with a getter func.
func NewFSCursorGetter(getter func(ctx context.Context) (FSCursor, error)) *FSCursorGetter {
	return &FSCursorGetter{getter: getter}
}

// CheckReleased checks if the fscursor is released without locking anything.
func (f *FSCursorGetter) CheckReleased() bool {
	return f.released.Load()
}

// GetCursorOps returns the interface implementing FSCursorGetterOps.
// Called after AddChangeCb and only if GetProxyCursor returns nil, nil.
// Return nil, nil to indicate this position is null (nothing here).
// Return nil, ErrReleased to indicate this FSCursorGetter was released.
func (f *FSCursorGetter) GetCursorOps(ctx context.Context) (FSCursorOps, error) {
	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	return nil, nil
}

// GetProxyCursor returns the value from the getter, if set.
func (f *FSCursorGetter) GetProxyCursor(ctx context.Context) (FSCursor, error) {
	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	if f.getter == nil {
		return nil, unixfs_errors.ErrNotExist
	}
	value, err := f.getter(ctx)
	if value == nil && err == nil {
		err = unixfs_errors.ErrNotExist
	}
	return value, err
}

// Release releases the filesystem cursor.
func (f *FSCursorGetter) Release() {
	f.released.Store(true)
}

// AddChangeCb is not applicable.
func (f *FSCursorGetter) AddChangeCb(cb FSCursorChangeCb) {}

// _ is a type assertion
var _ FSCursor = ((*FSCursorGetter)(nil))
