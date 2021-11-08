package unixfs

import (
	"context"
	"io/fs"
	"time"
)

// FSWriter coordinates writes to a filesystem tree.
// Methods should not return until the updated state has been synced to the FS.
// Updated state can be synced by setting the new root hash.
// Writers should be constructed one per FS object. Do not reuse.
// Mutex on fs is NOT locked by the caller.
type FSWriter interface {
	// FilesystemError is called when an internal error is encountered.
	FilesystemError(err error)
	// Mknod creates one or more inodes at the given paths.
	// An error may be returned if one or more parent directories don't exist.
	// ErrExist should be returned if one of the path entries exists with a different type.
	// Mkdir is implemented with Mknod.
	Mknod(ctx context.Context, paths [][]string, nodeType FSCursorNodeType, permissions uint32, ts time.Time) error
	// SetPermissions sets the permissions bits of the nodes at the paths.
	// The file mode portion of the value is ignored.
	SetPermissions(ctx context.Context, paths [][]string, fm fs.FileMode, ts time.Time) error
	// SetModTimestamp sets the modification timestamp of the nodes at the paths.
	SetModTimestamp(ctx context.Context, paths [][]string, ts time.Time) error
	// Write writes data to an offset in an inode (usually a file).
	Write(ctx context.Context, path []string, offset int64, data []byte, ts time.Time) error
	// Truncate shrinks or extends a file to the specified size.
	// The extended part will be a sparse range (hole) reading as zeros.
	Truncate(ctx context.Context, path []string, nsize int64, ts time.Time) error
	// Remove removes one or more paths from the tree.
	// Parents must be directories.
	// Non-existent paths may not return an error.
	Remove(ctx context.Context, paths [][]string, ts time.Time) error
}
