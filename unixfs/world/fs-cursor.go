package unixfs_world

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block_fs "github.com/aperturerobotics/hydra/unixfs/block/fs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/hydra/world"
	control "github.com/aperturerobotics/hydra/world/control"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// FSCursor allows attaching a cursor to a world object and watching for changes.
//   - FSObject (with changelog)
//   - FSNode (like inode)
//   - File (raw file block graph)
//
// A new cursor object is created for each position.
type FSCursor struct {
	// isReleased is an atomic bool indicating if this cursor is released
	isReleased atomic.Bool
	// le is the logger
	le *logrus.Entry
	// ws is the world state
	ws world.WorldState
	// posType is the current FSType of the position
	posType FSType
	// objKey is the world object key (if applicable)
	objKey string
	// writer is the fs writer
	writer unixfs.FSWriter
	// watchChanges indicates watching for changes is enabled
	watchChanges bool
	// bcast guards below fields
	bcast broadcast.Broadcast
	// prevObjRev is the previous revision id
	prevObjRev uint64
	// rootFSCursor is the current root fs cursor
	rootFSCursor unixfs.FSCursor
	// cbs is the set of change callbacks
	cbs unixfs.FSCursorChangeCbSlice
}

// NewFSCursor constructs a new FSCursor with a world object ref.
func NewFSCursor(
	le *logrus.Entry,
	ws world.WorldState,
	objKey string,
	posType FSType,
	writer unixfs.FSWriter,
	watchChanges bool,
) *FSCursor {
	return &FSCursor{
		le:           le,
		ws:           ws,
		objKey:       objKey,
		posType:      posType,
		writer:       writer,
		watchChanges: watchChanges,
	}
}

// CheckReleased checks if the fscursor is released without locking anything.
func (f *FSCursor) CheckReleased() bool {
	return f.isReleased.Load()
}

// WaitObjectRev waits for a world revision to be processed by watchWorldChanges.
// If the cursor becomes released, returns ErrReleased.
// If watchChanges is false, returns immediately.
// Waits for the world revision to be at least nrev.
// Can be used with SetConfirmFunc on the writer.
func (f *FSCursor) WaitObjectRev(ctx context.Context, nrev uint64) error {
	if !f.watchChanges {
		return nil
	}

	for {
		var wait <-chan struct{}
		f.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if f.prevObjRev >= nrev {
				wait = nil
			} else {
				wait = getWaitCh()
			}
		})
		if wait == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return context.Canceled
		case <-wait:
		}
	}
}

// GetCursorOps returns the interface implementing FSCursorOps.
// Return nil, nil to indicate this position is null (nothing here).
func (f *FSCursor) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	// never called
	return nil, nil
}

// GetProxyCursor returns a FSCursor to replace this one, if necessary.
// This is used to resolve a symbolic link, mount, etc.
// Return nil, nil if no redirection necessary (in most cases).
// This will be called before any of the other calls.
// Releasing a child cursor does not release the parent, and vise-versa.
// Return nil, ErrReleased if this FSCursor was released.
func (f *FSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	relFns := make([]func(), 0, 2)
	var fsc unixfs.FSCursor
	var retErr error

	f.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if f.rootFSCursor != nil {
			if f.rootFSCursor.CheckReleased() {
				f.rootFSCursor = nil
			} else {
				fsc = f.rootFSCursor
				return
			}
		}

		// initial state lookup
		objState, objFound, err := f.ws.GetObject(ctx, f.objKey)
		if !objFound {
			err = unixfs_errors.ErrNotExist
		}
		if err != nil {
			retErr = err
			return
		}

		objRef, objRev, err := objState.GetRootRef(ctx)
		if err != nil {
			retErr = err
			return
		}

		// build root cursor
		rootCursor, err := f.ws.BuildStorageCursor(ctx)
		if err != nil {
			// cannot build root cursor
			retErr = err
			return
		}
		relFns = append(relFns, rootCursor.Release)

		locCursor, err := rootCursor.FollowRef(ctx, objRef)
		if err != nil {
			retErr = err
			return
		}
		relFns = append(relFns, locCursor.Release)

		f.prevObjRev = objRev
		broadcast()

		switch f.posType {
		case FSType_FSType_FS_NODE:
			nfs := unixfs_block_fs.NewFS(ctx, 0, locCursor, f.writer)
			f.rootFSCursor = nfs
			// dispatch watch thread
			if f.watchChanges {
				go f.watchWorldChanges(nfs, objState, objRef)
			}
			// add callback to release cursors
			nfs.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
				if !ch.Released {
					return true
				}
				locCursor.Release()
				rootCursor.Release()
				return false
			})
			fsc = nfs
		default:
			retErr = errors.Errorf("TODO support pos type: %s", f.posType.String())
		}
	})

	if retErr != nil {
		for _, rel := range relFns {
			rel()
		}
		return nil, retErr
	}

	return fsc, nil
}

// AddChangeCb adds a change callback to detect when the cursor has changed.
// This will be called only if GetProxyCursor returns nil, nil.
//
// cb must not block, and should be called when cursor changes / is released
// cb will be called immediately (same call tree) if already released.
func (f *FSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	var added bool
	f.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		added = f.lockedAddChangeCb(cb)
	})
	if !added {
		cb(&unixfs.FSCursorChange{Cursor: f, Released: true})
	}
}

// lockedAddChangeCb calls AddChangeCb when rmtx is locked by caller.
// returns if the callback was added or not.
// the return value is !f.released
func (f *FSCursor) lockedAddChangeCb(cb unixfs.FSCursorChangeCb) bool {
	released := f.CheckReleased()
	if !released {
		f.cbs = append(f.cbs, cb)
	}
	return !released
}

// Release releases the filesystem cursor.
// note: locks mtx. must NOT be locked when calling
func (f *FSCursor) Release() {
	if f.CheckReleased() {
		return
	}
	f.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if f.isReleased.Swap(true) {
			return
		}
		if f.rootFSCursor != nil {
			if !f.rootFSCursor.CheckReleased() {
				f.rootFSCursor.Release()
			}
			f.rootFSCursor = nil
		}
	})
}

// watchWorldChanges waits for changes to the world object in a goroutine.
// started by GetProxyCursor
func (f *FSCursor) watchWorldChanges(nfs *unixfs_block_fs.FS, objState world.ObjectState, currRef *bucket.ObjectRef) {
	markLatestRev := func(rev uint64) {
		// proc any waiters
		f.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if f.prevObjRev != rev {
				f.prevObjRev = rev
				broadcast()
			}
		})
	}

	// handleWorldChange is called when the fs object changes in the world.
	var handleWorldChange control.WatchLoopHandler = func(
		ctx context.Context,
		le *logrus.Entry,
		world world.WorldState,
		obj world.ObjectState, // may be nil if not found
		rootRef *bucket.ObjectRef,
		rev uint64,
	) (waitForChanges bool, err error) {
		// if released, stop watching
		if f.CheckReleased() {
			return false, unixfs_errors.ErrReleased
		}
		// if no change, continue.
		if rootRef.GetRootRef().EqualsRef(currRef.GetRootRef()) {
			markLatestRev(rev)
			return true, nil
		}
		// if anything is different other than the root ref, release.
		if !rootRef.EqualsRefIgnoreRootRef(currRef) {
			nfs.Release()
			return false, nil
		}

		// apply the change
		currRef = rootRef
		if err := nfs.UpdateRootRef(ctx, rootRef.GetRootRef()); err != nil {
			return true, err
		}

		markLatestRev(rev)
		return true, nil
	}

	// pass nil for logger here
	objLoop := control.NewWatchLoop(nil, f.objKey, handleWorldChange)
	if err := objLoop.Execute(nfs.GetContext(), f.ws); err != nil {
		if err != context.Canceled && err != unixfs_errors.ErrReleased {
			f.le.WithError(err).Warn("error watching for world changes")
		}
	}

	// release root fs cursor
	nfs.Release()
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*FSCursor)(nil))
