package unixfs_world

import (
	"context"
	"io/fs"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/aperturerobotics/hydra/world"
)

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
}

// NewFSWriter constructs a new fs writer with a fscursor.
func NewFSWriter(ws world.WorldState, objKey string, fsType FSType, sender peer.ID) *FSWriter {
	return &FSWriter{ws: ws, objKey: objKey, fsType: fsType, sender: sender}
}

// FilesystemError is called when an internal error is encountered.
func (w *FSWriter) FilesystemError(err error) {
	// no-op for now
}

// Mknod creates one or more inodes at the given paths.
// An error may be returned if one or more parent directories don't exist.
// ErrExist should be returned if one of the path entries exists with a different type.
// Mkdir is implemented with Mknod.
func (w *FSWriter) Mknod(ctx context.Context, paths [][]string, nodeType unixfs.FSCursorNodeType, permissions uint32, ts time.Time) error {
	if len(paths) == 0 {
		return nil
	}

	wobj, err := w.getWorldObject(true)
	if err != nil {
		return err
	}
	return FsMknod(ctx, wobj, w.sender, w.fsType, paths, nodeType, permissions, ts)
}

// SetPermissions sets the permissions bits of the nodes at the paths.
// The file mode portion of the value is ignored.
func (w *FSWriter) SetPermissions(ctx context.Context, paths [][]string, fm fs.FileMode, ts time.Time) error {
	wobj, err := w.getWorldObject(true)
	if err != nil {
		return err
	}
	return FsSetPermissions(ctx, wobj, w.sender, w.fsType, paths, fm.Perm(), ts)
}

// SetModTimestamp sets the modification timestamp of the nodes at the paths.
func (w *FSWriter) SetModTimestamp(ctx context.Context, paths [][]string, ts time.Time) error {
	wobj, err := w.getWorldObject(true)
	if err != nil {
		return err
	}
	return FsSetModTimestamp(ctx, wobj, w.sender, w.fsType, paths, ts)
}

// Write writes data to an offset in an inode (usually a file).
func (w *FSWriter) Write(ctx context.Context, path []string, offset int64, data []byte, ts time.Time) error {
	wobj, err := w.getWorldObject(true)
	if err != nil {
		return err
	}
	return FsWrite(ctx, wobj, w.sender, w.fsType, path, offset, data, ts)
}

// Truncate shrinks or extends a file to the specified size.
// The extended part will be a sparse range (hole) reading as zeros.
func (w *FSWriter) Truncate(ctx context.Context, path []string, nsize int64, ts time.Time) error {
	wobj, err := w.getWorldObject(true)
	if err != nil {
		return err
	}
	return FsTruncate(ctx, wobj, w.sender, w.fsType, path, nsize, ts)
}

// Remove removes one or more paths from the tree.
// Parents must be directories.
// Non-existent paths may not return an error.
func (w *FSWriter) Remove(ctx context.Context, paths [][]string, ts time.Time) error {
	wobj, err := w.getWorldObject(true)
	if err != nil {
		return err
	}
	return FsRemove(ctx, wobj, w.sender, w.fsType, paths, ts)
}

// getWorldObject looks up the world fs object.
func (w *FSWriter) getWorldObject(checkExists bool) (world.ObjectState, error) {
	wobj, exists, err := w.ws.GetObject(w.objKey)
	if err == nil && !exists && checkExists {
		err = unixfs_errors.ErrNotExist
	}
	return wobj, err
}

// _ is a type assertion
var _ unixfs.FSWriter = ((*FSWriter)(nil))
