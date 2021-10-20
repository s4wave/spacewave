package unixfs_block

import (
	"context"
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
func (f *FSWriter) Mknod(ctx context.Context, paths [][]string, nodeType unixfs.FSCursorNodeType, permissions uint32, ts time.Time) error {
	nt := FSCursorNodeTypeToNodeType(nodeType)
	tts := ToTimestamp(ts, true)
	return Mknod(f.fsTree, paths, nt, permissions, tts)
}

// Write writes data to an offset in an inode (usually a file).
func (f *FSWriter) Write(ctx context.Context, path []string, offset int64, data []byte, ts time.Time) error {
	tts := ToTimestamp(ts, true)
	return Write(ctx, f.fsTree, nil, path, offset, data, tts)
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
