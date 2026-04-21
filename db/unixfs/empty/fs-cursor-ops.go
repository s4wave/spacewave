package unixfs_empty

import (
	"context"
	"io"
	"io/fs"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// FSCursorOps implements the FSCursorOps for an empty directory.
type FSCursorOps struct {
	cursor *FSCursor
}

func (e *FSCursorOps) CheckReleased() bool {
	return e.cursor.CheckReleased()
}

func (e *FSCursorOps) GetName() string {
	return ""
}

func (e *FSCursorOps) GetIsDirectory() bool {
	return true
}

func (e *FSCursorOps) GetIsFile() bool {
	return false
}

func (e *FSCursorOps) GetIsSymlink() bool {
	return false
}

func (e *FSCursorOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	return fs.ModeDir | 0o755, nil
}

func (e *FSCursorOps) SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error {
	return unixfs_errors.ErrReadOnly
}

func (e *FSCursorOps) GetSize(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (e *FSCursorOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	return time.Time{}, nil
}

func (e *FSCursorOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	return unixfs_errors.ErrReadOnly
}

func (e *FSCursorOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	return 0, unixfs_errors.ErrNotFile
}

func (e *FSCursorOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	return 0, unixfs_errors.ErrNotFile
}

func (e *FSCursorOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	return unixfs_errors.ErrReadOnly
}

func (e *FSCursorOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	return unixfs_errors.ErrReadOnly
}

func (e *FSCursorOps) Lookup(ctx context.Context, name string) (unixfs.FSCursor, error) {
	return nil, unixfs_errors.ErrNotExist
}

func (e *FSCursorOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	return nil // Empty directory, so no entries to return
}

func (e *FSCursorOps) Mknod(ctx context.Context, checkExist bool, names []string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	return unixfs_errors.ErrReadOnly
}

func (e *FSCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, targetIsAbsolute bool, ts time.Time) error {
	return unixfs_errors.ErrReadOnly
}

func (e *FSCursorOps) Readlink(ctx context.Context, name string) (pathNodes []string, isAbsolute bool, err error) {
	return nil, false, unixfs_errors.ErrNotSymlink
}

func (e *FSCursorOps) CopyTo(ctx context.Context, tgtDir unixfs.FSCursorOps, tgtName string, ts time.Time) (done bool, err error) {
	return false, nil
}

func (e *FSCursorOps) CopyFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (done bool, err error) {
	return false, unixfs_errors.ErrReadOnly
}

func (e *FSCursorOps) MoveTo(ctx context.Context, tgtCursorOps unixfs.FSCursorOps, tgtName string, ts time.Time) (done bool, err error) {
	return false, nil
}

func (e *FSCursorOps) MoveFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (done bool, err error) {
	return false, unixfs_errors.ErrReadOnly
}

func (e *FSCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	return unixfs_errors.ErrReadOnly
}

func (e *FSCursorOps) MknodWithContent(ctx context.Context, name string, nodeType unixfs.FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error {
	return unixfs_errors.ErrReadOnly
}
