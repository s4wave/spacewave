package space_unixfs

import (
	"context"
	"io"
	"io/fs"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// fsCursorOps implements the read-only operations of a projected directory.
type fsCursorOps struct {
	// cursor is the projected directory cursor.
	cursor *FSCursor
}

// CheckReleased reports whether the cursor was released.
func (o *fsCursorOps) CheckReleased() bool {
	return o == nil || o.cursor == nil || o.cursor.CheckReleased()
}

// GetName returns the name of the projected directory.
func (o *fsCursorOps) GetName() string {
	if o == nil || o.cursor == nil {
		return ""
	}
	return o.cursor.getName()
}

// GetIsDirectory reports true: every projected path is a directory.
func (o *fsCursorOps) GetIsDirectory() bool {
	return true
}

// GetIsFile reports false: a projected path is never a file.
func (o *fsCursorOps) GetIsFile() bool {
	return false
}

// GetIsSymlink reports false: a projected path is never a symlink.
func (o *fsCursorOps) GetIsSymlink() bool {
	return false
}

// GetPermissions returns mode 0755 for a projected directory.
func (o *fsCursorOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return 0o755, nil
}

// SetPermissions rejects the change: the projection is read-only.
func (o *fsCursorOps) SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// GetSize returns zero for a projected directory.
func (o *fsCursorOps) GetSize(ctx context.Context) (uint64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return 0, nil
}

// GetModTimestamp returns the zero time: a projected directory has no modification time.
func (o *fsCursorOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	if o.CheckReleased() {
		return time.Time{}, unixfs_errors.ErrReleased
	}
	return time.Time{}, nil
}

// SetModTimestamp rejects the change: the projection is read-only.
func (o *fsCursorOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// ReadAt rejects the read: a projected directory has no contents.
func (o *fsCursorOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return 0, unixfs_errors.ErrNotFile
}

// GetOptimalWriteSize rejects the query: the projection is read-only.
func (o *fsCursorOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return 0, unixfs_errors.ErrReadOnly
}

// WriteAt rejects the write: the projection is read-only.
func (o *fsCursorOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Truncate rejects the change: the projection is read-only.
func (o *fsCursorOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Lookup returns the cursor of the named child.
func (o *fsCursorOps) Lookup(ctx context.Context, name string) (unixfs.FSCursor, error) {
	if o.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	return o.cursor.lookupChild(ctx, name)
}

// ReaddirAll calls cb with each child after the first skip children.
func (o *fsCursorOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return o.cursor.readdirChildren(ctx, skip, cb)
}

// Mknod rejects the change: the projection is read-only.
func (o *fsCursorOps) Mknod(ctx context.Context, checkExist bool, names []string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Symlink rejects the change: the projection is read-only.
func (o *fsCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, targetIsAbsolute bool, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// Readlink rejects the read: a projected directory holds no symlinks.
func (o *fsCursorOps) Readlink(ctx context.Context, name string) ([]string, bool, error) {
	if o.CheckReleased() {
		return nil, false, unixfs_errors.ErrReleased
	}
	return nil, false, unixfs_errors.ErrNotSymlink
}

// CopyTo rejects the copy: the projection is read-only.
func (o *fsCursorOps) CopyTo(ctx context.Context, tgtDir unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	if o.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}
	return false, unixfs_errors.ErrReadOnly
}

// CopyFrom rejects the copy: the projection is read-only.
func (o *fsCursorOps) CopyFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	if o.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}
	return false, unixfs_errors.ErrReadOnly
}

// MoveTo rejects the move: the projection is read-only.
func (o *fsCursorOps) MoveTo(ctx context.Context, tgtCursorOps unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	if o.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}
	return false, unixfs_errors.ErrReadOnly
}

// MoveFrom rejects the move: the projection is read-only.
func (o *fsCursorOps) MoveFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	if o.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}
	return false, unixfs_errors.ErrReadOnly
}

// Remove rejects the change: the projection is read-only.
func (o *fsCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// MknodWithContent rejects the change: the projection is read-only.
func (o *fsCursorOps) MknodWithContent(ctx context.Context, name string, nodeType unixfs.FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// projectedDirent is one entry of a projected directory listing.
type projectedDirent struct {
	// name is the entry name.
	name string
	// nodeType is the entry node type.
	nodeType unixfs.FSCursorNodeType
}

// newProjectedDirent constructs a projected directory entry.
func newProjectedDirent(name string, nodeType unixfs.FSCursorNodeType) *projectedDirent {
	return &projectedDirent{name: name, nodeType: nodeType}
}

// GetName returns the entry name.
func (d *projectedDirent) GetName() string {
	return d.name
}

// GetIsDirectory reports whether the entry is a directory.
func (d *projectedDirent) GetIsDirectory() bool {
	return d.nodeType.GetIsDirectory()
}

// GetIsFile reports whether the entry is a file.
func (d *projectedDirent) GetIsFile() bool {
	return d.nodeType.GetIsFile()
}

// GetIsSymlink reports whether the entry is a symlink.
func (d *projectedDirent) GetIsSymlink() bool {
	return d.nodeType.GetIsSymlink()
}

// _ is a type assertion
var (
	_ unixfs.FSCursorOps    = (*fsCursorOps)(nil)
	_ unixfs.FSCursorDirent = (*projectedDirent)(nil)
)
