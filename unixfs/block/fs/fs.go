package unixfs_block_fs

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/block"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// OptimalWriteSize is a constant target size to use for Blob writes.
// currently set to default chunking max size * 2
const OptimalWriteSize = 512e3 * 2 // 512 KB * 2 = 1024KB ~= 1MB

// FS implements the unixfs FSCursor interfaces with a root Cursor.
type FS struct {
	// isReleased indicates if this is released.
	isReleased atomic.Bool
	// ctx is the root context for the fs tree
	ctx context.Context
	// ctxCancel cancels ctx
	ctxCancel context.CancelFunc
	// writer is the fs writer for this tree
	writer unixfs.FSWriter
	// rmtx guards below fields
	rmtx sync.Mutex
	// rootType is the type of the root cursor.
	// can be 0 to indicate any
	rootType unixfs_block.NodeType
	// rootCursor is the root lookup cursor
	rootCursor *bucket_lookup.Cursor
	// rootFSCursor is the root fs cursor.
	// check if nil or released before use
	rootFSCursor *FSCursor
	// cbs is the list of change callbacks
	// note: rmtx must be locked while calling
	cbs unixfs.FSCursorChangeCbSlice
}

// NewFS creates a new FS with a root lookup cursor and writer.
// the root cursor will be released when fs is released
// If the writer is nil, this tree is read-only.
func NewFS(
	ctx context.Context,
	rootType unixfs_block.NodeType,
	rootCursor *bucket_lookup.Cursor,
	writer unixfs.FSWriter,
) *FS {
	fs := &FS{
		rootType:   rootType,
		rootCursor: rootCursor,
		writer:     writer,
	}
	fs.ctx, fs.ctxCancel = context.WithCancel(ctx)
	return fs
}

// GetContext returns the context that is canceled when the fs is closed.
func (f *FS) GetContext() context.Context {
	return f.ctx
}

// CheckReleased checks if the fscursor is released without locking anything.
func (f *FS) CheckReleased() bool {
	return f.isReleased.Load()
}

// UpdateRootRef changes the root ref of the FS, canceling the resolution
// context and kicking off a new resolution pass down the tree.
func (f *FS) UpdateRootRef(blkRef *block.BlockRef) {
	if f.CheckReleased() {
		return
	}

	f.rmtx.Lock()
	defer f.rmtx.Unlock()

	if f.rootCursor.GetRef().GetRootRef().EqualsRef(blkRef) {
		// no changes
		return
	}
	f.rootCursor.SetRootRef(blkRef)

	if f.rootFSCursor == nil || f.rootFSCursor.CheckReleased() {
		return
	}
	if f.rootFSCursor.fsCursorOps == nil {
		return
	}
	_ = f.rootFSCursor.handleParentChanged(nil)
}

// GetProxyCursor returns a FSCursor to replace this one, if necessary.
// This is used to resolve a symbolic link, mount, etc.
// Return nil, nil if no redirection necessary (in most cases).
// This will be called before any of the other calls.
// Releasing a child cursor does not release the parent, and vise-versa.
// Return nil, ErrReleased if this FSCursor was released.
func (f *FS) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	// Build the root block-graph cursor.
	f.rmtx.Lock()
	defer f.rmtx.Unlock()
	return f.resolveRootFSCursor()
}

// AddChangeCb adds a change callback to detect when the cursor has changed.
//
// This will be called after GetProxyCursor returns nil, nil.
//
// cb must not block, and will be called when cursor changes / is released
//
// cb should /not/ be called immediately after AddChangeCb unless the cursor
// was already released, in which case it should be called exactly once.
func (f *FS) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	f.rmtx.Lock()
	_ = f.lockedAddChangeCb(cb)
	/*
		if f.lockedAddChangeCb(cb) {
			// ensure root fs cursor is resolved to give change callbacks
			_, _ = f.resolveRootFSCursor()
		}
	*/
	f.rmtx.Unlock()
}

// GetFSCursorOps returns the interface implementing FSCursorOps.
// Called after AddChangeCb and only if GetProxyCursor returns nil, nil.
// Return nil, nil to indicate this position is null (nothing here).
// Return nil, ErrReleased to indicate this FSCursor was released.
func (f *FS) GetFSCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	// no-op, this will never be called.
	return nil, nil
}

// Release releases the filesystem cursor.
func (f *FS) Release() {
	if f.CheckReleased() {
		return
	}
	f.ctxCancel()
	f.rmtx.Lock()
	defer f.rmtx.Unlock()
	if f.isReleased.Swap(true) {
		return
	}
	if f.rootFSCursor != nil {
		f.rootFSCursor.lockedRelease(true)
	}
	f.rootFSCursor = nil
	f.rootCursor.Release()
}

// lockedAddChangeCb calls AddChangeCb when rmtx is locked by caller.
// returns if the callback was added or not.
// the return value is !f.released
func (f *FS) lockedAddChangeCb(cb unixfs.FSCursorChangeCb) bool {
	released := f.CheckReleased()
	if !released {
		f.cbs = append(f.cbs, cb)
	}
	return !released
}

// resolveRootFSCursor gets/sets the rootFSCursor or returns an error
// caller must lock rmtx
func (f *FS) resolveRootFSCursor() (*FSCursor, error) {
	if f.rootFSCursor != nil {
		if f.rootFSCursor.CheckReleased() {
			f.rootFSCursor = nil
		} else {
			return f.rootFSCursor, nil
		}
	}

	rootNode, _, btx, err := f.buildRootTx()
	if err != nil {
		return nil, err
	}
	fsc := newFSCursor(f, nil, "", rootNode, btx)
	f.rootFSCursor = fsc
	fsc.lockedAddChangeCb(f.handleRootFSCursorChange)
	return f.rootFSCursor, nil
}

// handleRootFSCursorChange handles the root filesystem cursor changing.
func (f *FS) handleRootFSCursorChange(ch *unixfs.FSCursorChange) bool {
	if f.CheckReleased() {
		return false
	}
	// note: rmtx is locked
	if ch.Cursor != f.rootFSCursor {
		return false
	}
	if ch.Released {
		f.rootFSCursor = nil
		return false
	}
	ch = ch.Clone()
	ch.Cursor = f
	f.cbs = f.cbs.CallCbs(ch)
	return true
}

// buildRootTx builds a block transaction at the root of the tree.
// expects rmtx to be locked.
func (f *FS) buildRootTx() (*unixfs_block.FSTree, *block.Cursor, *block.Transaction, error) {
	if f.CheckReleased() {
		return nil, nil, nil, unixfs_errors.ErrReleased
	}

	// build fstree for the node
	btx, bcs := f.rootCursor.BuildTransaction(nil)
	root, err := unixfs_block.NewFSTree(f.ctx, bcs, f.rootType)
	return root, bcs, btx, err
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*FS)(nil))
