package unixfs

import (
	"context"
	"io"
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
	// Paths must be relative.
	Mknod(ctx context.Context, paths [][]string, nodeType FSCursorNodeType, permissions fs.FileMode, ts time.Time) error

	// Symlink creates a symbolic link from one location to another.
	// An error may be returned if one or more parent directories don't exist.
	// Supports absolute paths with targetIsAbsolute flag.
	Symlink(ctx context.Context, path []string, target []string, targetIsAbsolute bool, ts time.Time) error

	// SetPermissions sets the permissions bits of the nodes at the paths.
	// The file mode portion of the value is ignored.
	// Paths must be relative.
	SetPermissions(ctx context.Context, paths [][]string, fm fs.FileMode, ts time.Time) error

	// SetModTimestamp sets the modification timestamp of the nodes at the paths.
	// mtime is the modification timestamp to set.
	// Paths must be relative.
	SetModTimestamp(ctx context.Context, paths [][]string, mtime time.Time) error

	// WriteAt writes data to an offset in an inode (usually a file).
	// Must not retain data after returning.
	// Paths must be relative.
	WriteAt(ctx context.Context, path []string, offset int64, data []byte, ts time.Time) error

	// Truncate shrinks or extends a file to the specified size.
	// The extended part will be a sparse range (hole) reading as zeros.
	// Paths must be relative.
	// The file must already exist.
	Truncate(ctx context.Context, path []string, nsize int64, ts time.Time) error

	// Copy recursively copies a source path to a destination, overwriting destination.
	// Performs the move in a single operation.
	// Paths must be relative.
	Copy(ctx context.Context, srcPath, tgtPath []string, ts time.Time) error

	// Rename recursively moves a source path to a destination, overwriting destination.
	// Performs the move in a single operation.
	// Paths must be relative.
	Rename(ctx context.Context, srcPath, tgtPath []string, ts time.Time) error

	// Remove removes one or more paths from the tree.
	// Parents must be directories.
	// Non-existent paths may not return an error.
	// Paths must be relative.
	Remove(ctx context.Context, paths [][]string, ts time.Time) error

	// MknodWithContent creates a file and writes its content atomically.
	// The file appears fully formed in a single operation.
	// path is the full relative path to the new file.
	// dataLen is the total file size in bytes.
	// rdr provides the file content.
	// Path must be relative.
	MknodWithContent(ctx context.Context, path []string, nodeType FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error
}
