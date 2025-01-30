package unixfs_world

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/tx"
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
	// ctx is the context for watching for changes
	ctx context.Context
	// ctxCancel is canceled when the cursor is released
	ctxCancel context.CancelFunc
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
	ctx, ctxCancel := context.WithCancel(context.Background())
	return &FSCursor{
		ctx:       ctx,
		ctxCancel: ctxCancel,

		le:           le,
		ws:           ws,
		objKey:       objKey,
		posType:      posType,
		writer:       writer,
		watchChanges: watchChanges,
	}
}

// NewFSCursorWithWriter builds a FSCursor with a FSWriter setting the confirm func.
//
// watchChanges is always enabled so that WaitObjectRev can be notified of the changes.
func NewFSCursorWithWriter(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	objKey string,
	fsType FSType,
	sender peer.ID,
) (*FSCursor, *FSWriter) {
	// the fs writer processes write ops
	fsw := NewFSWriter(ws, objKey, fsType, sender)

	// construct the fs cursor
	// watchChanges must be true otherwise WaitObjectRev will never update
	fsc := NewFSCursor(le, ws, objKey, fsType, fsw, true)

	// we need the writer to wait until the FSCursor has processed the updated
	// revision of the world before returning from writes. pass the FSCursor to
	// the writer to set the additional wait function.
	fsw.SetConfirmFunc(fsc.WaitObjectRev)

	return fsc, fsw
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
	for {
		var wait <-chan struct{}
		var released bool
		f.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			released = f.CheckReleased()
			if !released && f.prevObjRev < nrev {
				wait = getWaitCh()
			}
		})
		if released {
			return unixfs_errors.ErrReleased
		}
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
		// check if the root fs cursor is valid & return it if so
		if f.rootFSCursor != nil {
			if f.rootFSCursor.CheckReleased() {
				f.rootFSCursor = nil
			} else {
				fsc = f.rootFSCursor
				return
			}
		}

		// lookup the object state
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

		// mark which revision we have & broadcast to wake waiters
		if f.prevObjRev < objRev {
			f.prevObjRev = objRev
			broadcast()
		}

		switch f.posType {
		case FSType_FSType_FS_NODE:
			nfs := unixfs_block_fs.NewFS(f.ctx, 0, locCursor, f.writer)
			f.rootFSCursor = nfs
			// dispatch goroutine to wait for changes
			if f.watchChanges {
				go func() {
					f.watchWorldChanges(nfs, objRef)
				}()
			}
			// add callback to release cursors when nfs is released
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
		// fast path
		return
	}
	var changeCbs unixfs.FSCursorChangeCbSlice
	f.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if f.isReleased.Swap(true) {
			return
		}
		f.ctxCancel()
		if f.rootFSCursor != nil {
			if !f.rootFSCursor.CheckReleased() {
				f.rootFSCursor.Release()
			}
			f.rootFSCursor = nil
		}
		changeCbs = f.cbs
		f.cbs = nil
		broadcast()
	})
	_ = changeCbs.CallCbs(&unixfs.FSCursorChange{Cursor: f, Released: true})
}

// watchWorldChanges waits for changes to the world object in a goroutine.
// started by GetProxyCursor
func (f *FSCursor) watchWorldChanges(nfs *unixfs_block_fs.FS, currRef *bucket.ObjectRef) {
	markLatestRev := func(rev uint64) {
		// proc any waiters
		f.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if f.prevObjRev < rev {
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
			return false, nil
		}

		switch {
		// if no change, keep the current fs.
		case rootRef.GetRootRef().EqualsRef(currRef.GetRootRef()):
			waitForChanges = true
		// if anything is different other than the root ref, release nfs.
		// this will result in GetProxyCursor being called for the next op.
		case !rootRef.EqualsRefIgnoreRootRef(currRef):
			nfs.Release()
		// otherwise, apply the change to the current cursor.
		default:
			currRef = rootRef
			waitForChanges = true
			err = nfs.UpdateRootRef(ctx, rootRef.GetRootRef())
		}

		// mark the latest revision
		markLatestRev(rev)

		return waitForChanges, err
	}

	// pass nil for logger here
	objLoop := control.NewWatchLoop(nil, f.objKey, handleWorldChange)
	ctx := nfs.GetContext()
	if err := objLoop.Execute(ctx, f.ws); err != nil {
		if err != context.Canceled && err != unixfs_errors.ErrReleased && err != tx.ErrDiscarded {
			f.le.WithError(err).Warn("error watching for world changes")
		}
	}

	// release root fs when loop exits
	nfs.Release()

	// release this cursor when loop exits
	// this signals to WaitObjectRev that watchWorldChanges is no longer running.
	f.Release()
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*FSCursor)(nil))
