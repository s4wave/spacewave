package unixfs

import (
	"context"
	"io"
	"io/fs"
	"time"
)

// FSCursor is a location in a filesystem tree.
// All operations should return ErrReleased if the cursor is released.
// The cursor can release itself if a complete cursor re-build is necessary.
type FSCursor interface {
	// CheckReleased checks if the fs cursor is currently released.
	CheckReleased() bool

	// GetProxyCursor returns a FSCursor to replace this one, if necessary.
	// This is used to resolve a symbolic link, mount, etc.
	// Return nil, nil if no redirection necessary (in most cases).
	// This will be called before any of the other calls.
	// Releasing a child cursor does not release the parent, and vise-versa.
	// The context may be canceled after GetProxyCursor returns and should not be retained.
	// Return nil, ErrReleased if this FSCursor was released.
	// Return nil, context.Canceled if ctx was canceled.
	GetProxyCursor(ctx context.Context) (FSCursor, error)

	// AddChangeCb adds a change callback to detect when the cursor has changed.
	// This will be called only if GetProxyCursor returns nil, nil.
	//
	// cb must not block, and should be called when cursor changes / is released
	// cb will be called immediately (same call tree) if already released.
	// note: the cursor may hold a mutex internally while calling cb, don't call Release inside the callback.
	AddChangeCb(cb FSCursorChangeCb)

	// GetCursorOps returns the FSCursorOps for the FSCursor.
	// The context may be canceled after GetProxyCursor returns and should not be retained.
	// Called after AddChangeCb and only if GetProxyCursor returns nil, nil.
	// Returning nil, nil will be corrected to nil, ErrNotExist.
	// Return nil, ErrReleased to indicate this FSCursor was released.
	// Return nil, context.Canceled if ctx was canceled.
	GetCursorOps(ctx context.Context) (FSCursorOps, error)

	// Release releases the filesystem cursor.
	Release()
}

// FSCursorNodeType indicates the type of node.
type FSCursorNodeType interface {
	// GetIsDirectory returns if the node is a directory.
	GetIsDirectory() bool
	// GetIsFile returns if the node is a regular file.
	GetIsFile() bool
	// GetIsSymlink returns if the node is a symlink.
	GetIsSymlink() bool
}

// FSCursorDirent is a directory entry.
type FSCursorDirent interface {
	// GetName returns the name of the directory entry.
	GetName() string
	// FSCursorNodeType indicates the type of dirent.
	FSCursorNodeType
}

// FSCursorOps are operations called against a non-proxy FSCursor.
// Operations return ErrReleased if the FSCursorOps was released.
// After release, the system will call GetCursorOps again.
// If the node type changes for any reason, the ops object should be released.
// All ops must be concurrency safe and may be called by multiple routines at once.
type FSCursorOps interface {
	// CheckReleased checks if the fs cursor ops object is currently released.
	// Note: this indicates if the FSCursorOps is released, not the parent FSCursor.
	CheckReleased() bool

	// GetName returns the name of the inode (if applicable).
	// i.e. directory name, filename.
	GetName() string

	// FSCursorNodeType indicates the type of dirent.
	FSCursorNodeType

	// GetPermissions returns the permissions bits of the file mode.
	// Only the permissions bits are set in the FileMode.
	GetPermissions(ctx context.Context) (fs.FileMode, error)

	// SetPermissions updates the permissions bits of the file mode.
	// Only the permissions bits are used from the FileMode.
	SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error

	// GetSize returns the size of the inode (in bytes).
	// Usually applicable only if this is a FILE.
	GetSize(ctx context.Context) (uint64, error)

	// GetModTimestamp returns the modification timestamp.
	GetModTimestamp(ctx context.Context) (time.Time, error)

	// SetModTimestamp updates the modification timestamp of the node.
	// mtime is the modification time to set.
	SetModTimestamp(ctx context.Context, mtime time.Time) error

	// ReadAt reads from a location in a File node.
	// This is similar to ReadAt from io.ReaderAt.
	//
	// When ReadAt returns n < len(data), it returns a non-nil error explaining
	// why more bytes were not returned. In this respect, ReadAt is stricter
	// than Read.
	//
	// Even if ReadAt returns n < len(data), it may use all of p as scratch
	// space during the call. If some data is available but not len(p) bytes,
	// ReadAt blocks until either all the data is available or an error occurs.
	// In this respect ReadAt is different from Read.
	//
	// If the n = len(data) bytes returned by ReadAt are at the end of the input
	// source, ReadAt may return either err == EOF or err == nil.
	//
	// If ReadAt is reading from an input source with a seek offset, ReadAt
	// should not affect nor be affected by the underlying seek offset.
	//
	// If this isn't a file node, returns ErrNotFile.
	//
	// Returns 0, io.EOF if the offset is past the end of the file.
	// Returns the length read and any error.
	ReadAt(ctx context.Context, offset int64, data []byte) (int64, error)

	// GetOptimalWriteSize returns the best write size to use for the Write call.
	// May return zero to indicate no known optimal size.
	GetOptimalWriteSize(ctx context.Context) (int64, error)

	// WriteAt writes to a location within a File node synchronously.
	// Accepts any size for the data parameter.
	// Call GetOptimalWriteSize to determine the best size of data to use.
	// The change should be fully written to the file before returning.
	// If this isn't a file node, returns ErrNotFile.
	WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error

	// Truncate shrinks or extends a file to the specified size.
	// The extended part will be a sparse range (hole) reading as zeros.
	Truncate(ctx context.Context, nsize uint64, ts time.Time) error

	// Lookup looks up a child entry in a directory.
	// Returns ErrNotExist if the child entry was not found.
	// Returns ErrReleased if the reference has been released.
	// Creates a new FSCursor at the new location.
	Lookup(ctx context.Context, name string) (FSCursor, error)

	// ReaddirAll reads all directory entries.
	// If skip is set, skips the first N directory entries.
	ReaddirAll(ctx context.Context, skip uint64, cb func(ent FSCursorDirent) error) error

	// Mknod creates child entries in a directory.
	// inode must be a directory.
	// if permissions is zero, default permissions will be set.
	// if checkExist, checks if name exists, returns ErrExist if so
	Mknod(ctx context.Context, checkExist bool, names []string, nodeType FSCursorNodeType, permissions fs.FileMode, ts time.Time) error

	// Symlink creates a symbolic link from a location to a path.
	Symlink(ctx context.Context, checkExist bool, name string, target []string, targetIsAbsolute bool, ts time.Time) error

	// Readlink reads a symbolic link contents.
	// If name is empty, reads the link at the cursor position.
	// Returns ErrNotSymlink if not a symbolic link.
	Readlink(ctx context.Context, name string) (pathNodes []string, isAbsolute bool, err error)

	// CopyTo performs an optimized copy of an dirent inode to another inode.
	// If the src is a directory, this should be a recursive copy.
	// If the destination already exists, this should clobber the destination (overwrite).
	// Callers should still check CopyFrom even if CopyTo is not implemented.
	// Returns false, nil if optimized copy to the target is not implemented.
	CopyTo(ctx context.Context, tgtDir FSCursorOps, tgtName string, ts time.Time) (done bool, err error)

	// CopyFrom performs an optimized copy from another inode.
	// If the src is a directory, this should be a recursive copy.
	// If the destination already exists, this should clobber the destination (overwrite).
	// Callers should still check CopyTo even if CopyFrom is not implemented.
	// Returns false, nil if optimized copy from the target is not implemented.
	CopyFrom(ctx context.Context, name string, srcCursorOps FSCursorOps, ts time.Time) (done bool, err error)

	// MoveTo performs an atomic and optimized move to another inode.
	// If the src is a directory, this should be a recursive copy.
	// If the destination already exists, this should clobber the destination (overwrite).
	// Callers should still check MoveFrom even if MoveTo is not implemented.
	//
	// In a single operation: overwrite the target fully with the source data,
	// and delete the source inode from its parent directory.
	//
	// Returns false, nil if atomic move to the target is not implemented.
	MoveTo(ctx context.Context, tgtCursorOps FSCursorOps, tgtName string, ts time.Time) (done bool, err error)

	// MoveFrom performs an atomic and optimized move from another inode.
	// If the src is a directory, this should be a recursive copy.
	// If the destination already exists, this should clobber the destination (overwrite).
	// Callers should still check MoveTo even if MoveFrom is not implemented.
	//
	// In a single operation: overwrite the inode fully with the target data,
	// and delete the target inode from its parent directory.
	//
	// Returns false, nil if atomic move from the target is not implemented.
	MoveFrom(ctx context.Context, name string, srcCursorOps FSCursorOps, ts time.Time) (done bool, err error)

	// Remove deletes entries from a directory.
	// Returns ErrReadOnly if read-only.
	// Does not return an error if they did not exist.
	Remove(ctx context.Context, names []string, ts time.Time) error

	// MknodWithContent creates a file entry and writes content atomically.
	// The inode must be a directory.
	// The new file appears fully formed with all content written.
	// dataLen is the total file size in bytes.
	// rdr provides the file content to write.
	// Each backend implements this differently:
	//   - Block backend: pre-builds blob, then atomic Mknod+WriteBlob in one commit.
	//   - Other backends: create file, then io.Copy the content.
	// Returns ErrReadOnly if the filesystem is read-only.
	MknodWithContent(ctx context.Context, name string, nodeType FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error
}

// FSCursorChangeCb is a callback function for a cursor change.
// Handles changes to the cursor.
// Return false to remove the callback handler.
type FSCursorChangeCb func(ch *FSCursorChange) bool

// FSCursorChangeCbSlice is a callback slice
type FSCursorChangeCbSlice []FSCursorChangeCb

// CallCbs calls all callbacks and deletes any if necessary.
func (cbs FSCursorChangeCbSlice) CallCbs(change *FSCursorChange) FSCursorChangeCbSlice {
	for i := 0; i < len(cbs); i++ {
		if !cbs[i](change) {
			// remove callback
			cbs[i] = cbs[len(cbs)-1]
			cbs[len(cbs)-1] = nil
			cbs = cbs[:len(cbs)-1]
			i--
		}
	}
	return cbs
}

// FSCursorChange is information about a change.
// If the offset and size is zero, handlers should completely flush inode cache.
type FSCursorChange struct {
	// Cursor is the FS cursor.
	Cursor FSCursor
	// Released indicates the cursor was released.
	Released bool
	// Offset is the location to flush from.
	Offset uint64
	// Size is the amount of data to flush.
	Size uint64
}

// Clone copies the FSCursorChange.
func (c *FSCursorChange) Clone() *FSCursorChange {
	if c == nil {
		return nil
	}
	return &FSCursorChange{
		Cursor:   c.Cursor,
		Released: c.Released,
		Offset:   c.Offset,
		Size:     c.Size,
	}
}
