package unixfs_access

import (
	"context"
	"slices"
	"sync"

	"github.com/s4wave/spacewave/db/unixfs"
)

// RotatingAccess provides a stable AccessUnixFSFunc whose backing provider can rotate.
type RotatingAccess struct {
	// mtx guards current and waiters
	mtx sync.Mutex
	// current is the active access function
	current AccessUnixFSFunc
	// waiters are release callbacks from active access attempts
	waiters []func()
}

// NewRotatingAccess constructs a blocked RotatingAccess.
func NewRotatingAccess() *RotatingAccess {
	return &RotatingAccess{
		current: newBlockedAccess(),
	}
}

// AccessUnixFS accesses the current UnixFS provider.
func (r *RotatingAccess) AccessUnixFS(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
	// Snapshot the UnixFS provider and register the access release callback.
	r.mtx.Lock()
	current := r.current
	r.waiters = append(r.waiters, released)
	r.mtx.Unlock()

	return current(ctx, released)
}

// SetCurrent sets the current UnixFS provider and releases waiting access attempts.
func (r *RotatingAccess) SetCurrent(fn AccessUnixFSFunc) {
	// Replace the UnixFS provider and detach its pending release callbacks.
	r.mtx.Lock()
	r.current = fn
	waiters := slices.Clone(r.waiters)
	r.waiters = nil
	r.mtx.Unlock()

	// Notify access attempts that their UnixFS provider changed.
	for _, waiter := range waiters {
		if waiter != nil {
			waiter()
		}
	}
}

// SetBlocked sets the current UnixFS provider to block until another provider is published.
func (r *RotatingAccess) SetBlocked() {
	r.SetCurrent(newBlockedAccess())
}

func newBlockedAccess() AccessUnixFSFunc {
	return func(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}
}
