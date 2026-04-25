package space_unixfs

import (
	"context"
	"io"
	"io/fs"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

type fsCursorOps struct {
	cursor *FSCursor
}

func (o *fsCursorOps) CheckReleased() bool {
	return o == nil || o.cursor == nil || o.cursor.CheckReleased()
}

func (o *fsCursorOps) GetName() string {
	if o == nil || o.cursor == nil {
		return ""
	}
	return o.cursor.getName()
}

func (o *fsCursorOps) GetIsDirectory() bool {
	return true
}

func (o *fsCursorOps) GetIsFile() bool {
	return false
}

func (o *fsCursorOps) GetIsSymlink() bool {
	return false
}

func (o *fsCursorOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return 0o755, nil
}

func (o *fsCursorOps) SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

func (o *fsCursorOps) GetSize(ctx context.Context) (uint64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return 0, nil
}

func (o *fsCursorOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	if o.CheckReleased() {
		return time.Time{}, unixfs_errors.ErrReleased
	}
	return time.Time{}, nil
}

func (o *fsCursorOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

func (o *fsCursorOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return 0, unixfs_errors.ErrNotFile
}

func (o *fsCursorOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}
	return 0, unixfs_errors.ErrReadOnly
}

func (o *fsCursorOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

func (o *fsCursorOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

func (o *fsCursorOps) Lookup(ctx context.Context, name string) (unixfs.FSCursor, error) {
	if o.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	return o.cursor.lookupChild(ctx, name)
}

func (o *fsCursorOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return o.cursor.readdirChildren(ctx, skip, cb)
}

func (o *fsCursorOps) Mknod(ctx context.Context, checkExist bool, names []string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

func (o *fsCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, targetIsAbsolute bool, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

func (o *fsCursorOps) Readlink(ctx context.Context, name string) ([]string, bool, error) {
	if o.CheckReleased() {
		return nil, false, unixfs_errors.ErrReleased
	}
	return nil, false, unixfs_errors.ErrNotSymlink
}

func (o *fsCursorOps) CopyTo(ctx context.Context, tgtDir unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	if o.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}
	return false, unixfs_errors.ErrReadOnly
}

func (o *fsCursorOps) CopyFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	if o.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}
	return false, unixfs_errors.ErrReadOnly
}

func (o *fsCursorOps) MoveTo(ctx context.Context, tgtCursorOps unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	if o.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}
	return false, unixfs_errors.ErrReadOnly
}

func (o *fsCursorOps) MoveFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	if o.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}
	return false, unixfs_errors.ErrReadOnly
}

func (o *fsCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

func (o *fsCursorOps) MknodWithContent(ctx context.Context, name string, nodeType unixfs.FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	return unixfs_errors.ErrReadOnly
}

// _ is a type assertion
var _ unixfs.FSCursorOps = ((*fsCursorOps)(nil))

type projectedChild struct {
	name string
}

type projectedDirent struct {
	name     string
	nodeType unixfs.FSCursorNodeType
}

func newProjectedDirent(name string, nodeType unixfs.FSCursorNodeType) *projectedDirent {
	return &projectedDirent{name: name, nodeType: nodeType}
}

func (d *projectedDirent) GetName() string {
	return d.name
}

func (d *projectedDirent) GetIsDirectory() bool {
	return d.nodeType.GetIsDirectory()
}

func (d *projectedDirent) GetIsFile() bool {
	return d.nodeType.GetIsFile()
}

func (d *projectedDirent) GetIsSymlink() bool {
	return d.nodeType.GetIsSymlink()
}

// _ is a type assertion
var _ unixfs.FSCursorDirent = ((*projectedDirent)(nil))
