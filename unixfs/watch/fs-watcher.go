package unixfs_watch

import (
	"context"
	"slices"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/util/broadcast"
)

// FSWatcherCb is a function called with the FSOps and FSCursor when the
// filesystem state changes.
//
// if fsError != nil, the passed fsPath may be a subset of fsTargetPath.
// The given path and handles will be for the parent of the errored path element.
// The passed fsHandles, fsCursor, and fsCursorOps are located at fsPath.
//
// fsHandles[0] is the root handle accessed by the access function.
// fsHandles[i+1] is the handle corresponding to fsTargetPath[i].
// If fsTargetPath is empty the fsHandles will still have len(1).
//
// If unixfs_errors.ErrReleased is returned, the function may be retried.
type FSWatcherCb func(
	ctx context.Context,
	fsTargetPath []string,
	fsError error,
	fsPath []string,
	fsHandles []*unixfs.FSHandle,
	fsCursor unixfs.FSCursor,
	fsOps unixfs.FSCursorOps,
) error

// FSWatcher watches a location and parents in a UnixFS with a FSHandle.
//
// Waits for the FSHandle to be resolved and for SetPath/SetPathPts to be called
// before calling the callback function.
type FSWatcher struct {
	cb     FSWatcherCb
	access unixfs_access.AccessUnixFSFunc

	// bcast is broadcast to wake up the Execute loop
	// guards below fields
	bcast broadcast.Broadcast
	// pathPts is the split path to lookup in the fs.
	// nil until resolved
	pathPts *[]string
	// handles are the current fs handles
	// index 0 is the location at access()
	// index 1->len(pathPts) correspond to pathPts
	handles []*unixfs.FSHandle
	// handleRel releases the handle
	handleRel func()
}

// NewFSWatcher constructs a new filesystem watcher with an access function.
func NewFSWatcher(cb FSWatcherCb, access unixfs_access.AccessUnixFSFunc) *FSWatcher {
	return &FSWatcher{cb: cb, access: access}
}

// Wake forces a re-check of the FSWatcher state.
func (w *FSWatcher) Wake() {
	w.bcast.HoldLockMaybeAsync(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		broadcast()
	})
}

// SetPath updates the path as a string.
// Returns the path split into parts and if the path changed or not.
// Triggers a re-check of the fs state if the path changed.
// Treats absolute paths as relative to the current directory.
// Note: do not modify the returned path slice.
// If the path is empty, the callback will receive the root node.
func (w *FSWatcher) SetPath(pathStr string) ([]string, bool) {
	pathPts, _ := unixfs.SplitPath(pathStr)
	return pathPts, w.SetPathPts(pathPts)
}

// SetPathPts updates the path as a pre-split path parts slice.
// Returns if the path changed or not.
// Triggers a re-check of the fs state if the path changed.
// Note: do not modify the passed path slice.
// If the path is empty, the callback will receive the root node.
func (w *FSWatcher) SetPathPts(pathPts []string) bool {
	var changed bool
	w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		// If the old path was empty, set & return.
		prevPathPtsPtr := w.pathPts
		if prevPathPtsPtr == nil {
			w.pathPts = &pathPts
			changed = true
			broadcast()
			return
		}

		// If the path didn't change, return.
		prevPathPts := *prevPathPtsPtr
		if !slices.Equal(prevPathPts, pathPts) {
			changed = true
		} else {
			return
		}

		// Compare the paths and drop entries that don't match.
		for i := len(prevPathPts) - 1; i >= 0; i-- {
			// If the previous path at i does not match the new path:
			if i >= len(pathPts) || pathPts[i] != prevPathPts[i] {
				// Release handle at this index + all after it.
				// Note that handles[0] is the access handle.
				// prevPathPts[i] corresponds to handles[i+1]
				// we need to release all handles from i+1 to end
				// reduce len(w.handles) to i+1
				for len(w.handles) > i+1 {
					w.handles[len(w.handles)-1].Release()
					w.handles[len(w.handles)-1] = nil
					w.handles = w.handles[:len(w.handles)-1]
				}
			}
		}

		// Update the pathPts
		w.pathPts = &pathPts
		broadcast()
	})
	return changed
}

// ClearPath clears the path and releases any path handles.
// The callback will be canceled and not called again until SetPath is called.
// Returns if there was previously a path set.
func (w *FSWatcher) ClearPath() bool {
	var changed bool
	w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		changed := w.pathPts != nil
		if !changed {
			return
		}

		w.pathPts = nil
		w.releaseLocked()
		broadcast()
	})
	return changed
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
		w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			w.releaseLocked()
			broadcast()
		})
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

		// currHandles is a copy of w.handles
		var currHandles []*unixfs.FSHandle
		var currPath *[]string
		w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			currHandles, currPath = w.getStateLocked()
			waitCh = getWaitCh()
			currHandles = slices.Clone(currHandles)
		})

		// if the path is not set yet, wait for SetPath to be called.
		if currPath == nil {
			continue
		}
		pathPts := *currPath // currPath != nil asserted 2 lines above

		// call the access function, if necessary.
		if len(currHandles) == 0 {
			// Call the access function & register the result.
			var rootHandle *unixfs.FSHandle
			var rootHandleRel func()
			rootHandle, rootHandleRel, err = w.access(ctx, func() {
				w.bcast.HoldLockMaybeAsync(func(broadcast func(), getWaitCh func() <-chan struct{}) {
					if len(w.handles) != 0 && w.handles[0] == rootHandle {
						// root handle was released already.
						w.handleRel = nil
						// lock the other handles
						w.releaseLocked()
					}
				})
			})
			if err != nil {
				return err
			}

			// Register the root handle.
			w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
				w.handles = []*unixfs.FSHandle{rootHandle}
				w.handleRel = rootHandleRel
			})

			// Update currHandles
			currHandles = append(currHandles, rootHandle)
		}

		// Traverse to the path until there are no handles remaining to traverse or fsError.
		var fsError error
		for len(currHandles) < len(pathPts)+1 {
			nextDir := pathPts[len(currHandles)-1]
			currHandle := currHandles[len(currHandles)-1]
			nextHandle, err := currHandle.Lookup(ctx, nextDir)
			if err != nil {
				// If something was released while performing the op, try again right away.
				if err == unixfs_errors.ErrReleased || err == context.Canceled {
					waitCh, waitCursorChanged = nil, nil
					continue
				}

				// The error must be some problem accessing this fs node.
				// Return the handles we got so far
				fsError = err
				break
			}

			// Register the new handle.
			// Ensure that nothing else has changed in the meantime.
			var valid bool
			w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
				valid = w.pathPts == currPath && len(w.handles) == len(currHandles)
				if valid {
					w.handles = append(w.handles, nextHandle)
					currHandles = append(currHandles, nextHandle)
				}
			})
			if !valid {
				// Something changed, try again.
				nextHandle.Release()
				continue
			}
		}

		// get the ops
		lookupHandle := currHandles[len(currHandles)-1]
		lookupHandleCursor, lookupHandleOps, err := lookupHandle.GetOps(ctx)
		if err != nil {
			// if something was released while performing the op, try again right away.
			if err == unixfs_errors.ErrReleased {
				continue
			}
			// otherwise return the error (we can't wait for changes if fsOps is nil)
			return err
		}

		// make sure ctx is still active and no errors
		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-errCh:
			return err
		default:
		}

		// wait for the cursor to be released
		// note: we will want to keep lookupHandle alive for this callback!
		nextWaitCursorChanged := make(chan struct{})
		lookupHandleCursor.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
			close(nextWaitCursorChanged)
			return false
		})
		waitCursorChanged = nextWaitCursorChanged

		// build the actual fsPath
		actualPathPts := make([]string, 0, len(pathPts))
		for _, handle := range currHandles[1:] {
			if name := handle.GetName(); name != "" {
				actualPathPts = append(actualPathPts, name)
			}
		}

		// call the callback
		err = w.cb(
			ctx,
			pathPts,
			fsError,
			actualPathPts,
			currHandles,
			lookupHandleCursor,
			lookupHandleOps,
		)
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
func (w *FSWatcher) getStateLocked() ([]*unixfs.FSHandle, *[]string) {
	// if the path is not set, drop all of the handles.
	if w.pathPts == nil {
		w.releaseLocked()
		return nil, w.pathPts
	}

	// drop all released handles
	for i := len(w.handles) - 1; i >= 0; i-- {
		if w.handles[i].CheckReleased() {
			w.handles[i] = nil
			w.handles = w.handles[:i]
		}
	}

	// if no handles remain, drop everything.
	if len(w.handles) == 0 {
		w.releaseLocked()
	}

	// return whatever handles we have
	return w.handles, w.pathPts
}

// releaseLocked releases the handles while locked.
func (w *FSWatcher) releaseLocked() {
	// release handles resolved while looking up PathPts
	if len(w.handles) > 1 {
		for _, handle := range w.handles[1:] {
			handle.Release()
		}
	}
	// drop the handles list
	w.handles = nil
	// release the root handle (returned by access())
	if w.handleRel != nil {
		w.handleRel()
		w.handleRel = nil
	}
}
