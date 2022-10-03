package unixfs_iofs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// FSCursorOps implements the filesystem ops.
type FSCursorOps struct {
	// FSCursorNodeType contains the node type.
	unixfs.FSCursorNodeType
	// isReleased is an atomic int indicating released
	isReleased atomic.Bool
	// cursor is the fs cursor
	cursor *FSCursor
	// fs is the root of the filesystem
	// lookups should use the path
	fs fs.FS
	// path is the path to this node
	path string
	// fileInfo is the information at the path
	fileInfo fs.FileInfo
	// dirents is the directory entry list, if a directory
	dirents []fs.DirEntry
}

// newFSCursorOps constructs a new FSCursorOps.
func newFSCursorOps(fsCursor *FSCursor, ifs fs.FS) (*FSCursorOps, error) {
	fpath := fsCursor.buildPathString()
	fileInfo, err := fs.Stat(ifs, fpath)
	if err != nil {
		return nil, err
	}
	ntype, err := unixfs.FileModeToNodeType(fileInfo.Mode())
	if err != nil {
		return nil, err
	}
	var dirents []fs.DirEntry
	if ntype.GetIsDirectory() {
		dirents, err = fs.ReadDir(ifs, fpath)
		if err != nil {
			return nil, err
		}
	}
	return &FSCursorOps{
		FSCursorNodeType: ntype,
		cursor:           fsCursor,
		fs:               ifs,
		path:             fpath,
		fileInfo:         fileInfo,
		dirents:          dirents,
	}, nil
}

// CheckReleased checks if the ops is released without locking anything
func (f *FSCursorOps) CheckReleased() bool {
	if f == nil {
		return true
	}
	return f.isReleased.Load()
}

// GetName returns the name of the inode (if applicable).
// i.e. directory name, filename.
func (f *FSCursorOps) GetName() string {
	return f.cursor.name
}

// GetSize returns the size of the inode (in bytes).
// Usually applicable only if this is a FILE.
func (f *FSCursorOps) GetSize(ctx context.Context) (uint64, error) {
	if f.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return uint64(f.fileInfo.Size()), nil
}

// GetModTimestamp returns the modification timestamp.
func (f *FSCursorOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	if f.CheckReleased() {
		return time.Time{}, unixfs_errors.ErrReleased
	}
	return f.fileInfo.ModTime(), nil
}

// SetModTimestamp updates the modification timestamp of the node.
func (f *FSCursorOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// GetPermissions returns the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (f *FSCursorOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	if f.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return f.fileInfo.Mode().Perm(), nil
}

// SetPermissions sets the permissions bits of the file mode.
// The file mode portion of the value is ignored.
func (f *FSCursorOps) SetPermissions(ctx context.Context, fm fs.FileMode, ts time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Read reads from an offset inside a file node.
func (f *FSCursorOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	if f.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	if !f.GetIsFile() {
		return 0, unixfs_errors.ErrNotFile
	}

	ff, err := f.fs.Open(f.path)
	if err != nil {
		return 0, err
	}
	defer ff.Close()

	switch f := ff.(type) {
	case io.ReaderAt:
		rn, err := f.ReadAt(data, offset)
		return int64(rn), err
	case io.Seeker:
		if offset != 0 {
			if _, err := f.Seek(int64(offset), io.SeekStart); err != nil {
				return 0, err
			}
		}
	default:
		// refuse if offset != 0
		if offset != 0 {
			return 0, errors.New("fs.FS: FSCursorOps: fs file must implement ReaderAt or Seeker")
		}
	}

	// read from the current location in the file handle
	rn, err := io.ReadAtLeast(ff, data, len(data))
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	return int64(rn), err
}

// GetOptimalWriteSize returns the best write size to use for the Write call.
// May return zero to indicate no known optimal size.
func (f *FSCursorOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	return 0, nil
}

// WriteAt writes to a location within a File node synchronously.
func (f *FSCursorOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Truncate shrinks or extends a file to the specified size.
func (f *FSCursorOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Lookup looks up a child entry in a directory.
// Returns ErrNotExist if the child entry was not found.
// Returns ErrReleased if the reference has been released.
// Creates a new FSCursor at the new location.
func (f *FSCursorOps) Lookup(ctx context.Context, name string) (unixfs.FSCursor, error) {
	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	// Search for the entry
	var dirent fs.DirEntry
	for _, ent := range f.dirents {
		if ent.Name() == name {
			dirent = ent
			break
		}
	}
	if dirent == nil {
		return nil, unixfs_errors.ErrNotExist
	}

	// Add this inode
	return f.cursor.buildChildCursor(name, dirent)
}

// ReaddirAll reads all directory entries to a callback.
func (f *FSCursorOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !f.GetIsDirectory() {
		return unixfs_errors.ErrNotDirectory
	}
	for i := int(skip); i < len(f.dirents); i++ {
		if err := cb(NewFSCursorDirent(f.dirents[i])); err != nil {
			return err
		}
	}
	return nil
}

// Mknod creates child entries in a directory.
// inode must be a directory.
// if checkExist, checks if name exists, returns ErrExist if so
func (f *FSCursorOps) Mknod(
	ctx context.Context,
	checkExist bool,
	names []string,
	nodeType unixfs.FSCursorNodeType,
	permissions fs.FileMode,
	ts time.Time,
) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Symlink creates a symbolic link from a location to a path.
func (f *FSCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, ts time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Readlink reads a symbolic link contents.
// If name is empty, reads the link at the cursor position.
// Returns ErrNotSymlink if not a symbolic link.
func (f *FSCursorOps) Readlink(ctx context.Context, name string) ([]string, error) {
	if !f.GetIsSymlink() {
		return nil, unixfs_errors.ErrNotSymlink
	}

	// Readlink is not currently supported by fs.FS
	// https://github.com/golang/go/issues/49580
	return nil, errors.New("io/fs: does not support symbolic links")
}

// CopyTo performs an optimized copy of an dirent inode to another inode.
func (f *FSCursorOps) CopyTo(
	ctx context.Context,
	tgtCursorOps unixfs.FSCursorOps,
	tgtName string,
	ts time.Time,
) (done bool, err error) {
	return false, nil
}

// CopyFrom performs an optimized copy from another inode.
func (f *FSCursorOps) CopyFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (done bool, err error) {
	return false, nil
}

// MoveTo performs an atomic and optimized move to another inode.
// If the src is a directory, this should be a recursive copy.
// Callers should still check MoveFrom even if MoveTo is not implemented.
//
// In a single operation: overwrite the target fully with the source data,
// and delete the source inode from its parent directory.
//
// Returns false, nil if atomic move to the target is not implemented.
func (f *FSCursorOps) MoveTo(
	ctx context.Context,
	tgtCursorOps unixfs.FSCursorOps,
	tgtName string,
	ts time.Time,
) (done bool, err error) {
	return false, nil
}

// MoveFrom performs an atomic and optimized move from another inode.
// If the src is a directory, this should be a recursive copy.
// Callers should still check MoveTo even if MoveFrom is not implemented.
//
// In a single operation: overwrite the inode fully with the target data,
// and delete the target inode from its parent directory.
//
// Returns false, nil if atomic move from the target is not implemented.
func (f *FSCursorOps) MoveFrom(ctx context.Context, name string, tgtCursorOps unixfs.FSCursorOps, ts time.Time) (done bool, err error) {
	return false, nil
}

// Remove deletes entries from a directory.
// Returns ErrReadOnly if read-only.
func (f *FSCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	if f.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// release marks the fscursorops as released.
func (f *FSCursorOps) release() {
	f.isReleased.Store(true)
}

// _ is a type assertion
var _ unixfs.FSCursorOps = ((*FSCursorOps)(nil))
