package unixfs_world

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block_fs "github.com/aperturerobotics/hydra/unixfs/block/fs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/hydra/world"
	control "github.com/aperturerobotics/hydra/world/control"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// FSCursor allows attaching a cursor to a world object and watching for changes.
//  - FSObject (with changelog)
//  - FSNode (like inode)
//  - File (raw file block graph)
// A new cursor object is created for each position.
type FSCursor struct {
	// isReleased is an atomic int indicating if this cursor is released
	isReleased uint32
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
	// mtx guards below fields
	mtx sync.Mutex
	// prevObjRef is the previous object reference
	prevObjRef *block.BlockRef
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
	return atomic.LoadUint32(&f.isReleased) == 1
}

// GetFSCursorOps returns the interface implementing FSCursorOps.
// Return nil, nil to indicate this position is null (nothing here).
func (f *FSCursor) GetFSCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
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

	// NOTE: be careful to unlock it below!
	f.mtx.Lock()

	if f.rootFSCursor != nil {
		if f.rootFSCursor.CheckReleased() {
			f.rootFSCursor = nil
		} else {
			f.mtx.Unlock()
			return f.rootFSCursor, nil
		}
	}

	// initial state lookup
	objState, objFound, err := f.ws.GetObject(f.objKey)
	if !objFound {
		err = unixfs_errors.ErrNotExist
	}
	if err != nil {
		f.mtx.Unlock()
		return nil, err
	}

	objRef, objRev, err := objState.GetRootRef()
	if err != nil {
		// cannot lookup the object ref
		f.mtx.Unlock()
		return nil, err
	}

	// build root cursor
	rootCursor, err := f.ws.BuildStorageCursor(ctx)
	if err != nil {
		// cannot build root cursor
		f.mtx.Unlock()
		return nil, err
	}

	locCursor, err := rootCursor.FollowRef(ctx, objRef)
	if err != nil {
		// cannot follow the object ref
		f.mtx.Unlock()
		rootCursor.Release()
		return nil, err
	}

	f.prevObjRef = objRef.GetRootRef()
	f.prevObjRev = objRev

	switch f.posType {
	case FSType_FSType_FS_NODE:
		nfs := unixfs_block_fs.NewFS(ctx, 0, locCursor, f.writer)
		f.rootFSCursor = nfs
		// dispatch watch thread
		if f.watchChanges {
			go f.watchWorldChanges(nfs, objState, objRef)
		}
		f.mtx.Unlock()
		// add callback to release cursors
		nfs.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
			if ch.Released {
				locCursor.Release()
				rootCursor.Release()
			}
			return !ch.Released
		})
		return nfs, err
	default:
		f.mtx.Unlock()
		locCursor.Release()
		rootCursor.Release()
		return nil, errors.Errorf("TODO support pos type: %s", f.posType.String())
	}
}

// AddChangeCb adds a change callback to detect when the cursor has changed.
// This will be called only if GetProxyCursor returns nil, nil.
//
// cb must not block, and should be called when cursor changes / is released
// cb will be called immediately (same call tree) if already released.
func (f *FSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	f.mtx.Lock()
	added := f.lockedAddChangeCb(cb)
	f.mtx.Unlock()
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

	f.mtx.Lock()
	if atomic.SwapUint32(&f.isReleased, 1) == 1 {
		// already released
		f.mtx.Unlock()
		return
	}
	if f.rootFSCursor != nil {
		if !f.rootFSCursor.CheckReleased() {
			f.rootFSCursor.Release()
		}
		f.rootFSCursor = nil
	}
	f.mtx.Unlock()
}

// watchWorldChanges waits for changes to the world object in a goroutine.
// started by GetProxyCursor
func (f *FSCursor) watchWorldChanges(nfs *unixfs_block_fs.FS, objState world.ObjectState, currRef *bucket.ObjectRef) {
	// handleWorldChange is called when the fs object changes in the world.
	var handleWorldChange control.ObjectLoopHandler = func(
		ctx context.Context,
		le *logrus.Entry,
		world world.WorldState,
		obj world.ObjectState, // may be nil if not found
		rootRef *bucket.ObjectRef, rev uint64,
	) (waitForChanges bool, err error) {
		// if released, stop watching
		if f.CheckReleased() {
			return false, unixfs_errors.ErrReleased
		}
		// if no change, continue.
		if rootRef.GetRootRef().EqualsRef(currRef.GetRootRef()) {
			return true, nil
		}
		// if anything is different other than the root ref, release.
		if !rootRef.EqualsRefIgnoreRootRef(currRef) {
			nfs.Release()
			return false, nil
		}

		// apply the change
		currRef = rootRef
		nfs.UpdateRootRef(rootRef.GetRootRef())
		return true, nil
	}

	// pass nil for logger here
	objLoop := control.NewObjectLoop(nil, f.ws, false, f.objKey, handleWorldChange)
	if err := objLoop.Execute(nfs.GetContext()); err != nil {
		if err != context.Canceled && err != unixfs_errors.ErrReleased {
			f.le.WithError(err).Warn("error watching for world changes")
		}
	}
	// release root fs cursor
	nfs.Release()
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*FSCursor)(nil))
