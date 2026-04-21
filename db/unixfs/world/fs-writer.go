package unixfs_world

import (
	"context"
	"io"
	"io/fs"
	"sync/atomic"
	"time"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/s4wave/spacewave/db/world"
)

// FSWriterConfirm is a function which confirms a write by waiting for an object
// revision to be processed. Returns any error waiting for nrev.
type FSWriterConfirm = func(ctx context.Context, nObjRev uint64) error

// FSWriter implements the writer by passing through operations to world ops.
type FSWriter struct {
	// ws is the world state
	ws world.WorldState
	// objKey is the object key
	objKey string
	// fsType is the filesystem object type
	fsType FSType
	// sender is the tx sender
	sender peer.ID
	// confirmFn is the confirm function
	// may be nil
	confirmFn atomic.Pointer[FSWriterConfirm]
}

// NewFSWriter constructs a new fs writer with a fscursor.
func NewFSWriter(ws world.WorldState, objKey string, fsType FSType, sender peer.ID) *FSWriter {
	return &FSWriter{ws: ws, objKey: objKey, fsType: fsType, sender: sender}
}

// SetConfirmFunc sets a function to be called to confirm fs writes by waiting
// for an updated object revision to be processed.
//
// Call before using the FSWriter.
func (w *FSWriter) SetConfirmFunc(fn FSWriterConfirm) {
	if fn != nil {
		w.confirmFn.Store(&fn)
	} else {
		w.confirmFn.Store(nil)
	}
}

// FilesystemError is called when an internal error is encountered.
func (w *FSWriter) FilesystemError(err error) {
	// no-op for now
}

// Mknod creates one or more inodes at the given paths.
// An error may be returned if one or more parent directories don't exist.
// ErrExist should be returned if one of the path entries exists with a different type.
// Mkdir is implemented with Mknod.
func (w *FSWriter) Mknod(ctx context.Context, paths [][]string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	if len(paths) == 0 {
		return nil
	}

	return w.applyConfirmOp(ctx, func(wobj world.ObjectState) (nrev uint64, sysErr bool, err error) {
		return FsMknod(ctx, wobj, w.sender, w.fsType, paths, nodeType, permissions, ts)
	})
}

// Symlink creates a symbolic link from a location to a path.
// An error may be returned if one or more parent directories don't exist.
func (w *FSWriter) Symlink(ctx context.Context, path []string, target []string, targetIsAbsolute bool, ts time.Time) error {
	if len(path) == 0 || len(target) == 0 {
		return unixfs_errors.ErrEmptyPath
	}

	return w.applyConfirmOp(ctx, func(wobj world.ObjectState) (nrev uint64, sysErr bool, err error) {
		return FsSymlink(ctx, wobj, w.sender, w.fsType, path, target, targetIsAbsolute, ts)
	})
}

// SetPermissions sets the permissions bits of the nodes at the paths.
// The file mode portion of the value is ignored.
func (w *FSWriter) SetPermissions(ctx context.Context, paths [][]string, fm fs.FileMode, ts time.Time) error {
	return w.applyConfirmOp(ctx, func(wobj world.ObjectState) (nrev uint64, sysErr bool, err error) {
		return FsSetPermissions(ctx, wobj, w.sender, w.fsType, paths, fm.Perm(), ts)
	})
}

// SetModTimestamp sets the modification timestamp of the nodes at the paths.
func (w *FSWriter) SetModTimestamp(ctx context.Context, paths [][]string, mtime time.Time) error {
	return w.applyConfirmOp(ctx, func(wobj world.ObjectState) (nrev uint64, sysErr bool, err error) {
		return FsSetModTimestamp(ctx, wobj, w.sender, w.fsType, paths, mtime)
	})
}

// WriteAt writes data to an offset in an inode (usually a file).
func (w *FSWriter) WriteAt(ctx context.Context, path []string, offset int64, data []byte, ts time.Time) error {
	return w.applyConfirmOp(ctx, func(wobj world.ObjectState) (nrev uint64, sysErr bool, err error) {
		return FsWriteAt(ctx, wobj, w.sender, w.fsType, path, offset, data, ts)
	})
}

// Truncate shrinks or extends a file to the specified size.
// The extended part will be a sparse range (hole) reading as zeros.
func (w *FSWriter) Truncate(ctx context.Context, path []string, nsize int64, ts time.Time) error {
	return w.applyConfirmOp(ctx, func(wobj world.ObjectState) (nrev uint64, sysErr bool, err error) {
		return FsTruncate(ctx, wobj, w.sender, w.fsType, path, nsize, ts)
	})
}

// Copy recursively copies a source path to a destination, overwriting destination.
// Performs the move in a single operation.
// Called on the FS containing the /source/ inode, not the destination inode.
func (w *FSWriter) Copy(ctx context.Context, srcPath, tgtPath []string, ts time.Time) error {
	// TODO: process mountpoints
	// TODO: optimized copy between two block-backed FS in the same bucket + transform config.

	// The FSWriter is usually called when someone called an operation on the
	// block graph backed FS ops object. Usually this happens when both of the
	// locations are on the same world object FS.
	return w.applyConfirmOp(ctx, func(wobj world.ObjectState) (nrev uint64, sysErr bool, err error) {
		return FsCopy(ctx, w.ws, w.sender, w.objKey, w.fsType, srcPath, tgtPath, ts)
	})
}

// Rename recursively moves a source path to a destination, overwriting destination.
// Performs the move in a single operation.
func (w *FSWriter) Rename(ctx context.Context, srcPath, tgtPath []string, ts time.Time) error {
	// TODO: process mountpoints
	// TODO: optimized rename between two block-backed FS in the same bucket + transform config.

	// The FSWriter is usually called when someone called an operation on the
	// block graph backed FS ops object. Usually this happens when both of the
	// locations are on the same world object FS.
	return w.applyConfirmOp(ctx, func(wobj world.ObjectState) (nrev uint64, sysErr bool, err error) {
		return FsRename(ctx, w.ws, w.sender, w.objKey, w.fsType, srcPath, tgtPath, ts)
	})
}

// Remove removes one or more paths from the tree.
// Parents must be directories.
// Non-existent paths may not return an error.
func (w *FSWriter) Remove(ctx context.Context, paths [][]string, ts time.Time) error {
	return w.applyConfirmOp(ctx, func(wobj world.ObjectState) (nrev uint64, sysErr bool, err error) {
		return FsRemove(ctx, wobj, w.sender, w.fsType, paths, ts)
	})
}

// MknodWithContent creates a file and writes content atomically.
// Pre-builds the blob, then applies the mknod+content op in a single commit.
func (w *FSWriter) MknodWithContent(ctx context.Context, path []string, nodeType unixfs.FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error {
	return w.applyConfirmOp(ctx, func(wobj world.ObjectState) (nrev uint64, sysErr bool, err error) {
		return FsMknodWithContent(ctx, wobj, w.sender, w.fsType, path, nodeType, dataLen, rdr, permissions, ts)
	})
}

// getWorldObject looks up the world fs object.
func (w *FSWriter) getWorldObject(ctx context.Context, checkExists bool) (world.ObjectState, error) {
	wobj, exists, err := w.ws.GetObject(ctx, w.objKey)
	if err == nil && !exists && checkExists {
		err = unixfs_errors.ErrNotExist
	}
	return wobj, err
}

// applyConfirmOp gets the world object, applies the op, and confirms it (if applicable)
func (w *FSWriter) applyConfirmOp(ctx context.Context, op func(wobj world.ObjectState) (nrev uint64, sysErr bool, err error)) error {
	wobj, err := w.getWorldObject(ctx, true)
	if err != nil {
		return err
	}

	nrev, _, err := op(wobj)
	if err != nil {
		return err
	}

	if confirmFn := w.confirmFn.Load(); confirmFn != nil {
		if err := (*confirmFn)(ctx, nrev); err != nil {
			return err
		}
	}

	return nil
}

// _ is a type assertion
var _ unixfs.FSWriter = ((*FSWriter)(nil))
