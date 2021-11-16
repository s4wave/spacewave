package unixfs

import (
	"context"
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
	// Return nil, ErrReleased if this FSCursor was released.
	GetProxyCursor(ctx context.Context) (FSCursor, error)

	// AddChangeCb adds a change callback to detect when the cursor has changed.
	// This will be called only if GetProxyCursor returns nil, nil.
	//
	// cb must not block, and should be called when cursor changes / is released
	// cb will be called immediately (same call tree) if already released.
	AddChangeCb(cb FSCursorChangeCb)

	// GetFSCursorOps returns the interface implementing FSCursorOps.
	// Called after AddChangeCb and only if GetProxyCursor returns nil, nil.
	// Returning nil, nil will be corrected to nil, ErrNotExist.
	// Return nil, ErrReleased to indicate this FSCursor was released.
	GetFSCursorOps(ctx context.Context) (FSCursorOps, error)

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
// After release, the system will call GetFSCursorOps again.
// If the node type changes for any reason, the ops object should be released.
// All ops must be concurrency safe and may be called by multiple routines at once.
type FSCursorOps interface {
	// CheckReleased checks if the fs cursor ops object is currently released.
	// Note: this does not necessarily mean the FSCursor is released.
	CheckReleased() bool
	// GetName returns the name of the inode (if applicable).
	// i.e. directory name, filename.
	GetName() string

	// FSCursorNodeType indicates the type of dirent.
	FSCursorNodeType

	// GetPermissions returns the permissions bits of the file mode.
	// The file mode portion of the value is ignored.
	GetPermissions(ctx context.Context) (fs.FileMode, error)

	// SetPermissions updates the permissions bits of the file mode.
	// The file mode portion of the value is ignored.
	SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error

	// GetSize returns the size of the inode (in bytes).
	// Usually applicable only if this is a FILE.
	GetSize(ctx context.Context) (uint64, error)

	// GetModTimestamp returns the modification timestamp.
	GetModTimestamp(ctx context.Context) (time.Time, error)

	// SetModTimestamp updates the modification timestamp of the node.
	SetModTimestamp(ctx context.Context, ts time.Time) error

	// Read reads from a location in a File node.
	// If this isn't a file node, returns ErrNotFile.
	// Returns 0, io.EOF if the offset is past the end of the file.
	// Returns the length read and any error.
	Read(ctx context.Context, offset int64, data []byte) (int64, error)

	// GetOptimalWriteSize returns the best write size to use for the Write call.
	// May return zero to indicate no known optimal size.
	GetOptimalWriteSize(ctx context.Context) (int64, error)

	// Write writes to a location within a File node synchronously.
	// Accepts any size for the data parameter.
	// Call GetOptimalWriteSize to determine the best size of data to use.
	// The change should be fully written to the file before returning.
	// If this isn't a file node, returns ErrNotFile.
	Write(ctx context.Context, offset int64, data []byte, ts time.Time) error

	// Truncate shrinks or extends a file to the specified size.
	// The extended part will be a sparse range (hole) reading as zeros.
	Truncate(ctx context.Context, nsize uint64, ts time.Time) error

	// Lookup looks up a child entry in a directory.
	// Returns ErrNotExist if the child entry was not found.
	// Returns ErrReleased if the reference has been released.
	// Creates a new FSCursor at the new location.
	Lookup(ctx context.Context, name string) (FSCursor, error)

	// ReaddirAll reads all directory entries.
	ReaddirAll(ctx context.Context, cb func(ent FSCursorDirent) error) error

	// Mknod creates child entries in a directory.
	// inode must be a directory.
	// if permissions is zero, default permissions will be set.
	// if checkExist, checks if name exists, returns ErrExist if so
	Mknod(ctx context.Context, checkExist bool, names []string, nodeType FSCursorNodeType, permissions fs.FileMode, ts time.Time) error

	// Symlink creates a symbolic link from a location to a path.
	Symlink(ctx context.Context, checkExist bool, name string, target []string, ts time.Time) error

	// Readlink reads a symbolic link contents.
	// If name is empty, reads the link at the cursor position.
	// Returns ErrNotSymlink if not a symbolic link.
	Readlink(ctx context.Context, name string) ([]string, error)

	// Remove deletes entries from a directory.
	// Returns ErrReadOnly if read-only.
	// Does not return an error if they did not exist.
	Remove(ctx context.Context, names []string, ts time.Time) error

	// TODO
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
