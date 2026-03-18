package unixfs_block

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"time"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/hydra/unixfs"
)

// FSWriter implements the writer against a block cursor.
type FSWriter struct {
	// fsTree is the root of the filesystem tree
	fsTree *FSTree
}

// NewFSWriter builds the filesystem writer.
func NewFSWriter(fsTree *FSTree) *FSWriter {
	return &FSWriter{fsTree: fsTree}
}

// FilesystemError is called when an internal error is encountered.
func (f *FSWriter) FilesystemError(err error) {
	//  noop
}

// Mknod creates one or more inodes at the given paths.
// An error may be returned if one or more parent directories don't exist.
// ErrExist should be returned if one of the path entries exists with a different type.
// Mkdir is implemented with Mknod.
func (f *FSWriter) Mknod(ctx context.Context, paths [][]string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	nt := FSCursorNodeTypeToNodeType(nodeType)
	tts := ToTimestamp(ts, true)
	return Mknod(f.fsTree, paths, nt, permissions, tts)
}

// Symlink creates a symbolic link from a location to a path.
// An error may be returned if one or more parent directories don't exist.
func (f *FSWriter) Symlink(ctx context.Context, path []string, target []string, isAbsolute bool, ts time.Time) error {
	tts := ToTimestamp(ts, true)
	lnk := NewFSSymlink(NewFSPath(target, isAbsolute))
	_, err := Symlink(f.fsTree, path, lnk, tts)
	return err
}

// SetPermissions sets the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (f *FSWriter) SetPermissions(ctx context.Context, paths [][]string, fm fs.FileMode, ts time.Time) error {
	tts := ToTimestamp(ts, true)
	return SetPermissions(f.fsTree, paths, fm, tts)
}

// SetModTimestamp sets the modification timestamp of the file.
func (f *FSWriter) SetModTimestamp(ctx context.Context, paths [][]string, mtime time.Time) error {
	tts := ToTimestamp(mtime, false)
	return SetModTimestamp(f.fsTree, paths, tts)
}

// Write writes data to an offset in an inode (usually a file).
func (f *FSWriter) WriteAt(ctx context.Context, path []string, offset int64, data []byte, ts time.Time) error {
	tts := ToTimestamp(ts, true)
	return WriteAt(ctx, f.fsTree, nil, path, offset, int64(len(data)), bytes.NewReader(data), tts)
}

// Copy recursively copies a source path to a destination, overwriting destination.
// Performs the move in a single operation.
func (f *FSWriter) Copy(ctx context.Context, srcPath, tgtPath []string, ts time.Time) error {
	tts := ToTimestamp(ts, true)
	return CopyOrRename(f.fsTree, srcPath, tgtPath, false, tts)
}

// Rename moves an inode from a source path to a destination path.
// Overwrites the destination path.
func (f *FSWriter) Rename(ctx context.Context, srcPath, tgtPath []string, ts time.Time) error {
	tts := ToTimestamp(ts, true)
	return CopyOrRename(f.fsTree, srcPath, tgtPath, true, tts)
}

// Truncate shrinks or extends a file to the specified size.
// The extended part will be a sparse range (hole) reading as zeros.
func (f *FSWriter) Truncate(ctx context.Context, path []string, nsize int64, ts time.Time) error {
	tts := ToTimestamp(ts, true)
	return TruncateFile(ctx, f.fsTree, path, nsize, tts)
}

// Remove removes one or more paths from the tree.
// Parents must be directories.
// Non-existent paths may not return an error.
func (f *FSWriter) Remove(ctx context.Context, paths [][]string, ts time.Time) error {
	tts := ToTimestamp(ts, true)
	_, err := Remove(f.fsTree, paths, tts)
	return err
}

// MknodWithContent creates a file and writes content atomically.
// Builds the blob inline and writes it to the new file entry.
func (f *FSWriter) MknodWithContent(ctx context.Context, path []string, nodeType unixfs.FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error {
	nt := FSCursorNodeTypeToNodeType(nodeType)
	tts := ToTimestamp(ts, true)

	// Build blob in a detached transaction.
	blobCs := f.fsTree.GetCursor().DetachTransaction()
	blobCs.SetRefAtCursor(nil, true)
	_, err := blob.BuildBlob(ctx, dataLen, rdr, blobCs, nil)
	if err != nil {
		return err
	}

	// Write the detached transaction to flush blocks and compute the blob ref.
	// Without this, GetRef() returns nil because the block has not been hashed.
	blobRef, _, err := blobCs.GetTransaction().Write(ctx, true)
	if err != nil {
		return err
	}
	if blobRef == nil {
		blobRef = &block.BlockRef{}
	}

	return MknodWithContent(ctx, f.fsTree, path, nt, permissions, tts, blobRef)
}

// _ is a type assertion
var _ unixfs.FSWriter = ((*FSWriter)(nil))
