package unixfs_block_fs

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/block"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/util/csync"
)

// OptimalWriteSize is a constant target size to use for Blob writes.
// currently set to default chunking max size * 2
const OptimalWriteSize = 512e3 * 2 // 512 KB * 2 = 1024KB ~= 1MB

// FS implements the unixfs FSCursor interfaces with a root Cursor.
//
// The FSWriter is called with any write operations.
type FS struct {
	// isReleased indicates if this is released.
	isReleased atomic.Bool
	// ctx is the root context for the fs tree
	ctx context.Context
	// ctxCancel cancels ctx
	ctxCancel context.CancelFunc
	// rmtx is the read/write mutex for the FS tree and below fields.
	rmtx csync.RWMutex
	// writer is the fs writer for this tree
	writer unixfs.FSWriter
	// rootType is the type of the root cursor.
	// can be 0 to indicate any
	rootType unixfs_block.NodeType
	// bls is the root bucket lookup cursor
	bls *bucket_lookup.Cursor
	// rootFSCursor is the root fs cursor.
	// check if nil or released before use
	rootFSCursor *FSCursor
	// cbs is the list of change callbacks
	cbs unixfs.FSCursorChangeCbSlice
}

// NewFS creates a new FS with a root lookup cursor and writer.
// the root cursor will be released when fs is released
// If the writer is nil, this tree is read-only.
func NewFS(
	ctx context.Context,
	rootType unixfs_block.NodeType,
	bls *bucket_lookup.Cursor,
	writer unixfs.FSWriter,
) *FS {
	fs := &FS{
		rootType: rootType,
		bls:      bls,
		writer:   writer,
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

// UpdateRootRef changes the root ref of the FS.
func (f *FS) UpdateRootRef(ctx context.Context, blkRef *block.BlockRef) error {
	rel, err := f.rmtx.Lock(ctx, true)
	if err != nil {
		return err
	}
	defer rel()

	return f.updateRootRefLocked(blkRef)
}

// updateRootRefLocked updates the root fs ref while the mtx is write locked.
func (f *FS) updateRootRefLocked(blkRef *block.BlockRef) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	if f.bls.GetRef().GetRootRef().EqualsRef(blkRef) {
		// no changes
		return nil
	}

	f.bls.SetRootRef(blkRef)

	if f.rootFSCursor == nil || f.rootFSCursor.CheckReleased() {
		f.rootFSCursor = nil
		return nil
	}

	// release the cursors
	// this also releases child cursors (which subscribe to AddChangeCb)
	f.rootFSCursor.releaseLocked()
	f.rootFSCursor = nil

	return nil
}

// GetProxyCursor returns a FSCursor to replace this one, if necessary.
// This is used to resolve a symbolic link, mount, etc.
// Return nil, nil if no redirection necessary (in most cases).
// This will be called before any of the other calls.
// Releasing a child cursor does not release the parent, and vise-versa.
// Return nil, ErrReleased if this FSCursor was released.
func (f *FS) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	rel, err := f.rmtx.Lock(ctx, true)
	if err != nil {
		return nil, err
	}
	defer rel()

	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	// Build the root block-graph cursor.
	return f.resolveRootFSCursorLocked()
}

// AddChangeCb adds a change callback to detect when the cursor has changed.
//
// This will be called after GetProxyCursor returns nil, nil.
//
// cb must not block, and will be called when cursor changes / is released
func (f *FS) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	var added bool
	rel, err := f.rmtx.Lock(f.ctx, true)
	if err == nil {
		added = f.addChangeCbLocked(cb)
		rel()
	}
	if !added {
		cb(&unixfs.FSCursorChange{Released: true})
	}
}

// GetCursorOps returns the interface implementing FSCursorOps.
// Called after AddChangeCb and only if GetProxyCursor returns nil, nil.
// Return nil, nil to indicate this position is null (nothing here).
// Return nil, ErrReleased to indicate this FSCursor was released.
func (f *FS) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	// no-op, this will never be called.
	return nil, nil
}

// Release releases the filesystem cursor.
func (f *FS) Release() {
	if f.isReleased.Swap(true) {
		return
	}
	f.ctxCancel()
	rel, err := f.rmtx.Lock(context.Background(), true)
	if err != nil {
		return
	}
	if f.rootFSCursor != nil {
		f.rootFSCursor.releaseLocked()
	}
	f.rootFSCursor = nil
	f.bls.Release()
	rel()
}

// addChangeCbLocked calls AddChangeCb while locked.
// returns if the callback was added or not.
// the return value is !f.released
func (f *FS) addChangeCbLocked(cb unixfs.FSCursorChangeCb) bool {
	released := f.CheckReleased()
	if !released {
		f.cbs = append(f.cbs, cb)
	}
	return !released
}

// resolveRootFSCursorLocked gets/sets the rootFSCursor or returns an error
func (f *FS) resolveRootFSCursorLocked() (*FSCursor, error) {
	if f.rootFSCursor != nil {
		if f.rootFSCursor.CheckReleased() {
			f.rootFSCursor = nil
		} else {
			return f.rootFSCursor, nil
		}
	}

	rootNode, _, btx, err := f.buildRootTxLocked()
	if err != nil {
		return nil, err
	}
	fsc := newFSCursor(f, nil, "", rootNode, btx)
	f.rootFSCursor = fsc
	fsc.addChangeCbLocked(f.handleRootFSCursorChangeLocked)
	return f.rootFSCursor, nil
}

// handleRootFSCursorChangeLocked handles the root filesystem cursor changing.
func (f *FS) handleRootFSCursorChangeLocked(ch *unixfs.FSCursorChange) bool {
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
	// root fs cursor changed: it will emit an event to any listeners.
	return true
}

// buildRootTx builds a block transaction at the root of the tree.
func (f *FS) buildRootTxLocked() (*unixfs_block.FSTree, *block.Cursor, *block.Transaction, error) {
	if f.CheckReleased() {
		return nil, nil, nil, unixfs_errors.ErrReleased
	}

	// build fstree for the node
	btx, bcs := f.bls.BuildTransaction(nil)
	root, err := unixfs_block.NewFSTree(f.ctx, bcs, f.rootType)
	return root, bcs, btx, err
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*FS)(nil))
