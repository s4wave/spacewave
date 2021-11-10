package unixfs_block_fs

import (
	"context"
	"io/fs"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	"github.com/aperturerobotics/timestamp"
)

// FSWriter implements a writer on top of a FS.
type FSWriter struct {
	// fs is the target fs
	fs *FS
	// ts is the timestamp
	ts *timestamp.Timestamp
}

// NewFSWriter is a writer which applies changes to a rootCursor.
// Note: call SetFS before writer is used.
func NewFSWriter() *FSWriter {
	return &FSWriter{}
}

// SetFS sets the filesystem.
func (f *FSWriter) SetFS(fs *FS) {
	f.fs = fs
}

// SetTimestamp sets the timestamp to use for ops.
func (f *FSWriter) SetTimestamp(ts *timestamp.Timestamp) {
	f.ts = ts
}

// FilesystemError is called when an internal error is encountered.
func (f *FSWriter) FilesystemError(err error) {
	// noop
}

// Mknod creates one or more inodes at the given paths.
// An error may be returned if one or more parent directories don't exist.
// ErrExist should be returned if one of the path entries exists with a different type.
// Mkdir is implemented with Mknod.
func (f *FSWriter) Mknod(ctx context.Context, paths [][]string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	if len(paths) == 0 {
		return nil
	}
	return f.applyOp(ctx, func(ft *unixfs_block.FSTree, wr *unixfs_block.FSWriter) error {
		return wr.Mknod(ctx, paths, nodeType, permissions, ts)
	})
}

// SetPermissions sets the permissions bits of the nodes at the paths.
// The file mode portion of the value is ignored.
func (f *FSWriter) SetPermissions(ctx context.Context, paths [][]string, fm fs.FileMode, ts time.Time) error {
	if len(paths) == 0 {
		return nil
	}
	return f.applyOp(ctx, func(ft *unixfs_block.FSTree, wr *unixfs_block.FSWriter) error {
		return wr.SetPermissions(ctx, paths, fm, ts)
	})
}

// SetModTimestamp sets the modification timestamp of the nodes at the paths.
func (f *FSWriter) SetModTimestamp(ctx context.Context, paths [][]string, ts time.Time) error {
	if len(paths) == 0 {
		return nil
	}
	return f.applyOp(ctx, func(ft *unixfs_block.FSTree, wr *unixfs_block.FSWriter) error {
		return wr.SetModTimestamp(ctx, paths, ts)
	})
}

// Write writes data to an offset in an inode (usually a file).
func (f *FSWriter) Write(ctx context.Context, path []string, offset int64, data []byte, ts time.Time) error {
	return f.applyOp(ctx, func(ft *unixfs_block.FSTree, wr *unixfs_block.FSWriter) error {
		return wr.Write(ctx, path, offset, data, ts)
	})
}

// Truncate shrinks or extends a file to the specified size.
// The extended part will be a sparse range (hole) reading as zeros.
func (f *FSWriter) Truncate(ctx context.Context, path []string, nsize int64, ts time.Time) error {
	return f.applyOp(ctx, func(ft *unixfs_block.FSTree, wr *unixfs_block.FSWriter) error {
		return wr.Truncate(ctx, path, nsize, ts)
	})
}

// Remove removes one or more paths from the tree.
// Parents must be directories.
// Non-existent paths may not return an error.
func (f *FSWriter) Remove(ctx context.Context, paths [][]string, ts time.Time) error {
	return f.applyOp(ctx, func(ft *unixfs_block.FSTree, wr *unixfs_block.FSWriter) error {
		return wr.Remove(ctx, paths, ts)
	})
}

// applyOp applies an operation to the store.
func (f *FSWriter) applyOp(
	ctx context.Context,
	cb func(ft *unixfs_block.FSTree, wr *unixfs_block.FSWriter) error,
) error {
	// build root tx
	f.fs.rmtx.Lock()
	fsTree, bcs, btx, err := f.fs.buildRootTx()
	f.fs.rmtx.Unlock()
	if err == nil {
		wr := unixfs_block.NewFSWriter(fsTree)
		err = cb(fsTree, wr)
	}
	if err != nil {
		return err
	}

	oldRoot := bcs.GetRef()
	nroot, _, err := btx.Write(true)
	if err != nil {
		return err
	}
	if !nroot.EqualsRef(oldRoot) {
		f.fs.UpdateRootRef(nroot)
	}
	return nil
}

// _ is a type assertion
var _ unixfs.FSWriter = ((*FSWriter)(nil))
