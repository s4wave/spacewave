package unixfs_block

import (
	"bytes"
	"context"
	"io/fs"
	"time"

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
func (f *FSWriter) Symlink(ctx context.Context, path []string, target []string, ts time.Time) error {
	tts := ToTimestamp(ts, true)
	lnk := NewFSSymlink(NewFSPath(target))
	return Symlink(f.fsTree, path, lnk, tts)
}

// SetPermissions sets the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (f *FSWriter) SetPermissions(ctx context.Context, paths [][]string, fm fs.FileMode, ts time.Time) error {
	tts := ToTimestamp(ts, true)
	return SetPermissions(f.fsTree, paths, fm, tts)
}

// SetModTimestamp sets the modification timestamp of the file.
func (f *FSWriter) SetModTimestamp(ctx context.Context, paths [][]string, ts time.Time) error {
	tts := ToTimestamp(ts, false)
	return SetModTimestamp(f.fsTree, paths, tts)
}

// Write writes data to an offset in an inode (usually a file).
func (f *FSWriter) Write(ctx context.Context, path []string, offset int64, data []byte, ts time.Time) error {
	tts := ToTimestamp(ts, true)
	return Write(ctx, f.fsTree, nil, path, offset, int64(len(data)), bytes.NewReader(data), tts)
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

// _ is a type assertion
var _ unixfs.FSWriter = ((*FSWriter)(nil))
