package unixfs_world

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/s4wave/spacewave/db/world"
	control "github.com/s4wave/spacewave/db/world/control"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// FSCursor attaches a cursor to a World object and watches it for changes.
//
// It supports these positions:
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
	// watchBcast guards watchers and wakes Release as each watcher exits.
	watchBcast broadcast.Broadcast
	// watchers counts the world-change watchers still running, which Release
	// waits for before it returns.
	watchers int
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
	return NewFSCursorWithContext(context.Background(), le, ws, objKey, posType, writer, watchChanges)
}

// NewFSCursorWithContext constructs a new FSCursor whose watchers stop when ctx is canceled.
// The filesystem reads blocks with ctx, so a block.AccessLogger on ctx logs
// every block read through the cursor.
func NewFSCursorWithContext(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	objKey string,
	posType FSType,
	writer unixfs.FSWriter,
	watchChanges bool,
) *FSCursor {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, ctxCancel := context.WithCancel(ctx)
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
	return NewFSCursorWithWriterContext(context.Background(), le, ws, objKey, fsType, sender)
}

// NewFSCursorWithWriterContext builds a writer-backed FSCursor whose watchers stop when ctx is canceled.
func NewFSCursorWithWriterContext(
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
	fsc := NewFSCursorWithContext(ctx, le, ws, objKey, fsType, fsw, true)

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

// GetChangeCh returns a channel closed at the next change to the cursor: an
// applied World object revision, a new root, or release.
func (f *FSCursor) GetChangeCh() <-chan struct{} {
	var ch <-chan struct{}
	f.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
	})
	return ch
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
	// Reuse the root filesystem cursor while it is valid.
	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	var fsc unixfs.FSCursor
	f.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		fsc = f.lockedRootFSCursor()
	})
	if fsc != nil {
		return fsc, nil
	}
	if f.posType != FSType_FSType_FS_NODE {
		return nil, errors.Errorf("TODO support pos type: %s", f.posType.String())
	}

	// Open the object's storage outside bcast, so revision waiters, change
	// callbacks and Release do not wait on block reads.
	root, err := f.openRoot(ctx)
	if err != nil {
		return nil, err
	}

	// Install the filesystem unless the cursor was released or another call
	// installed one first.
	var released, installed bool
	f.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Keep the released state or the filesystem installed first.
		if f.CheckReleased() {
			released = true
			return
		}
		if fsc = f.lockedRootFSCursor(); fsc != nil {
			return
		}

		// Install the filesystem over the opened root.
		installed = true
		fsc = f.lockedInstallRoot(root)

		// Mark the observed revision and wake its waiters.
		if f.prevObjRev < root.rev {
			f.prevObjRev = root.rev
			broadcast()
		}
	})
	if !installed {
		root.release()
	}
	if released {
		return nil, unixfs_errors.ErrReleased
	}
	return fsc, nil
}

// lockedRootFSCursor returns the root filesystem cursor if it is valid.
// The caller holds bcast.
func (f *FSCursor) lockedRootFSCursor() unixfs.FSCursor {
	if f.rootFSCursor != nil && f.rootFSCursor.CheckReleased() {
		f.rootFSCursor = nil
	}
	return f.rootFSCursor
}

// fsRoot is an opened World object root: its reference, revision, and the
// storage cursor positioned at it.
type fsRoot struct {
	// ref is the World object reference.
	ref *bucket.ObjectRef
	// rev is the World object revision.
	rev uint64
	// cursor is the storage cursor following ref.
	cursor *bucket_lookup.Cursor
	// release releases cursor and the storage it reads.
	release func()
}

// openRoot reads the World object and opens its storage cursor.
func (f *FSCursor) openRoot(ctx context.Context) (*fsRoot, error) {
	// Read the World object reference and revision.
	objState, objFound, err := f.ws.GetObject(ctx, f.objKey)
	defer world.ReleaseObjectState(objState)
	if err == nil && !objFound {
		err = unixfs_errors.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	objRef, objRev, err := objState.GetRootRef(ctx)
	if err != nil {
		return nil, err
	}

	// Stage the filesystem's writes until the writer publishes them.
	stage, err := world.OpenWorldStorage(ctx, f.ws)
	if err != nil {
		return nil, err
	}

	// Follow the World object reference through the stage.
	rootCursor, err := stage.BuildStorageCursor(ctx)
	if err != nil {
		stage.Release()
		return nil, err
	}
	locCursor, err := rootCursor.FollowRef(ctx, objRef)
	if err != nil {
		rootCursor.Release()
		stage.Release()
		return nil, err
	}
	return &fsRoot{
		ref:    objRef,
		rev:    objRev,
		cursor: locCursor,
		release: func() {
			locCursor.Release()
			rootCursor.Release()
			stage.Release()
		},
	}, nil
}

// lockedInstallRoot builds the root filesystem over root and starts watching
// the World object. The filesystem releases root when it is released.
// The caller holds bcast.
func (f *FSCursor) lockedInstallRoot(root *fsRoot) unixfs.FSCursor {
	// Build the filesystem and follow the object's changes.
	nfs := unixfs_block_fs.NewFS(f.ctx, 0, root.cursor, f.writer)
	f.rootFSCursor = nfs
	if f.watchChanges {
		f.startWatcher(func() {
			f.watchWorldChanges(nfs, root.ref)
		})
	}

	// Release the root with the filesystem.
	nfs.AddChangeCb(func(ch *unixfs.FSCursorChange) bool {
		if !ch.Released {
			return true
		}
		root.release()
		return false
	})
	return nfs
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

// Release releases the filesystem cursor and waits for its watchers.
// The caller must not hold bcast.
func (f *FSCursor) Release() {
	f.release(true)
}

func (f *FSCursor) release(waitForWatchers bool) {
	// Stop an already released FSCursor after its watchers finish when requested.
	if f.CheckReleased() {
		// fast path
		if waitForWatchers {
			f.waitWatchers()
		}
		return
	}

	// Release the FSCursor state and detach its change callbacks under the lock.
	var changeCbs unixfs.FSCursorChangeCbSlice
	f.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		// Cancel the FSCursor watchers and release its cached root cursor.
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

	// Notify detached callbacks and finish the FSCursor watchers when requested.
	_ = changeCbs.CallCbs(&unixfs.FSCursorChange{Cursor: f, Released: true})
	if waitForWatchers {
		f.waitWatchers()
	}
}

// startWatcher runs fn on its own goroutine, counted in watchers until it
// returns.
func (f *FSCursor) startWatcher(fn func()) {
	// Count the watcher before it starts, so a Release that follows waits.
	l := f.watchBcast.Lock()
	f.watchers++
	l.Unlock()

	// Run it, and wake the waiters when it exits.
	go func() {
		defer func() {
			l := f.watchBcast.Lock()
			f.watchers--
			l.Broadcast()
			l.Unlock()
		}()
		fn()
	}()
}

// waitWatchers waits until every world-change watcher returned. It holds
// watchBcast only to read the count, never while it waits.
func (f *FSCursor) waitWatchers() {
	for {
		l := f.watchBcast.Lock()
		if f.watchers == 0 {
			l.Unlock()
			return
		}
		waitCh := l.WaitCh()
		l.Unlock()
		<-waitCh
	}
}

// watchWorldChanges waits for changes to the world object in a goroutine.
// started by GetProxyCursor
func (f *FSCursor) watchWorldChanges(nfs *unixfs_block_fs.FS, currRef *bucket.ObjectRef) {
	// Prepare a revision notifier for World change waiters.
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
	handleWorldChange := func(
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
	f.release(false)
}

// _ is a type assertion
var _ unixfs.FSCursor = (*FSCursor)(nil)
