package unixfs_watch

import (
	"context"
	"sync"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/util/broadcast"
	"golang.org/x/exp/slices"
)

// FSWatcherCb is a function called with the FSOps and FSCursor when the
// filesystem state changes.
//
// if fsError != nil, the passed fsPath may be a subset of fsTargetPath.
// The given path and handles will be for the parent of the errored path element.
// The passed fsHandle, fsCursor, and fsCursorOps are located at fsPath.
//
// If unixfs_errors.ErrReleased is returned, the function may be retried.
type FSWatcherCb func(
	ctx context.Context,
	fsTargetPath []string,
	fsError error,
	fsPath []string,
	fsHandle *unixfs.FSHandle,
	fsCursor unixfs.FSCursor,
	fsOps unixfs.FSCursorOps,
) error

// FSWatcher watches a location in a UnixFS with a FSHandle.
//
// Waits for the FSHandle to be resolved and for SetPath/SetPathPts to be called
// before calling the callback function.
type FSWatcher struct {
	cb     FSWatcherCb
	access unixfs_access.AccessUnixFSFunc

	// mtx guards below fields
	mtx sync.Mutex
	// bcast is broadcasted when the below changes
	bcast broadcast.Broadcast
	// pathPts is the split path to lookup in the fs.
	// nil until resolved
	pathPts *[]string
	// handle is the current fs handle
	handle *unixfs.FSHandle
	// handleRel releases handle
	handleRel func()
}

// NewFSWatcher constructs a new filesystem watcher with an access function.
func NewFSWatcher(cb FSWatcherCb, access unixfs_access.AccessUnixFSFunc) *FSWatcher {
	return &FSWatcher{cb: cb, access: access}
}

// Wake forces a re-check of the FSWatcher state.
func (w *FSWatcher) Wake() {
	w.bcast.Broadcast()
}

// SetPath updates the path as a string.
// Returns the path split into parts and if the path changed or not.
// Triggers a re-check of the fs state if the path changed.
// Treats absolute paths as relative to the current directory.
// Note: do not modify the returned path slice.
func (w *FSWatcher) SetPath(pathStr string) ([]string, bool) {
	pathPts, _ := unixfs.SplitPath(pathStr)
	return pathPts, w.SetPathPts(pathPts)
}

// SetPathPts updates the path as a pre-split path parts slice.
// Returns if the path changed or not.
// Triggers a re-check of the fs state if the path changed.
// Note: do not modify the passed path slice.
func (w *FSWatcher) SetPathPts(pathPts []string) bool {
	w.mtx.Lock()
	var unchanged bool
	if oldPathPtsPtr := w.pathPts; oldPathPtsPtr != nil {
		oldPathPts := *oldPathPtsPtr
		unchanged = slices.Equal(oldPathPts, pathPts)
	}
	if !unchanged {
		w.pathPts = &pathPts
		w.bcast.Broadcast()
	}
	w.mtx.Unlock()
	return !unchanged
}

// Execute executes the FSWatcher routine.
// Returns on any fatal error (if accessFn returns an error).
// Releases handles when returning.
// If the callback returns any error other than ErrReleased, returns that error.
// errCh is an optional error channel to interrupt execution. can be nil.
func (w *FSWatcher) Execute(rctx context.Context, errCh <-chan error) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer func() {
		ctxCancel()
		w.mtx.Lock()
		if w.handleRel != nil {
			w.handleRel()
			w.handleRel = nil
		}
		w.handle = nil
		w.bcast.Broadcast()
		w.mtx.Unlock()
	}()

	// re-check when any of the following change:
	// - the access function is released
	// - the handle returned by accessFn is released
	// - the path changes (w.bcast is broadcast)
	var err error
	var waitCh <-chan struct{}
	var waitCursorChanged <-chan struct{}
	for {
		// wait for changes if necessary
		if waitCh != nil || waitCursorChanged != nil {
			select {
			case <-ctx.Done():
				return context.Canceled
			case err := <-errCh:
				return err
			case <-waitCh:
			case <-waitCursorChanged:
			}
			waitCh, waitCursorChanged = nil, nil //nolint:ineffassign
		} else {
			select {
			case <-ctx.Done():
				return context.Canceled
			case err := <-errCh:
				return err
			default:
			}
		}

		w.mtx.Lock()
		currHandle, currPath := w.getStateLocked()
		waitCh = w.bcast.GetWaitCh()
		w.mtx.Unlock()

		if currPath == nil {
			// wait for the path to be set
			continue
		}

		// call the access function, if necessary.
		if currHandle == nil {
			var nextHandle *unixfs.FSHandle
			var nextHandleRel func()
			nextHandle, nextHandleRel, err = w.access(ctx, func() {
				rel := func(lock bool) {
					if lock {
						w.mtx.Lock()
					}
					if w.handle == nextHandle {
						w.handle = nil
						w.bcast.Broadcast()
					}
					w.mtx.Unlock()
				}

				// avoid deadlock
				if w.mtx.TryLock() {
					rel(false)
				} else {
					go rel(true)
				}
			})
			if err != nil {
				return err
			}
			w.mtx.Lock()
			w.handle, w.handleRel = nextHandle, nextHandleRel
			w.bcast.Broadcast()
			w.mtx.Unlock()
			continue
		}

		// traverse to the path
		// note: we assert currPath != nil above
		var fsError error
		pathPts := *currPath
		lookupHandle, lookupHandlePath, err := currHandle.LookupPathPts(ctx, pathPts)
		if err != nil {
			// if something was released while performing the op, try again right away.
			if err == unixfs_errors.ErrReleased || err == context.Canceled {
				continue
			}
			if lookupHandle != nil {
				// gracefully handle the error, pass it to the callback.
				// it may be ErrNotExist or ErrNotDirectory or similar.
				fsError = err
			} else {
				// error is not handled here, return it.
				return err
			}
		}

		// get the ops
		lookupHandleCursor, lookupHandleOps, err := lookupHandle.GetOps(ctx)
		if err != nil {
			// if something was released while performing the op, try again right away.
			if err == unixfs_errors.ErrReleased {
				continue
			}
			// otherwise return the error (we can't wait if the cursor is nil)
			return err
		}

		// make sure ctx is still active
		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-errCh:
			return err
		default:
		}

		// wait for the cursor to be released
		nextWaitCursorChanged := make(chan struct{})
		lookupHandleCursor.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
			close(nextWaitCursorChanged)
			return false
		})
		waitCursorChanged = nextWaitCursorChanged

		// call the callback
		err = w.cb(ctx, pathPts, fsError, lookupHandlePath, lookupHandle, lookupHandleCursor, lookupHandleOps)
		if err != nil {
			if err == unixfs_errors.ErrReleased {
				// clear the wait channels & try again right away.
				waitCh, waitCursorChanged = nil, nil
				continue
			}
			// return the error if it's not ErrReleased
			return err
		}
	}
}

// getStateLocked returns the current state while mtx is locked by the caller.
// returns the current fs handle and path string
// releases the handle if the path is not currently set.
// unsets the handle if the handle is released.
func (w *FSWatcher) getStateLocked() (*unixfs.FSHandle, *[]string) {
	// release the handle if the path is not set or if the handle is already released.
	if w.handle != nil && (w.pathPts == nil || w.handle.CheckReleased()) {
		w.handle.Release()
		w.handle = nil
		if w.handleRel != nil {
			go w.handleRel()
			w.handleRel = nil
		}
		w.bcast.Broadcast()
	}
	return w.handle, w.pathPts
}
