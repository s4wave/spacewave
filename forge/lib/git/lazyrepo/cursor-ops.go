package forge_lib_git_lazyrepo

import (
	"context"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// FSCursorOps wraps read-mode ops and redirects to allocated writable roots.
type FSCursorOps struct {
	cursor *FSCursor
	base   unixfs.FSCursorOps
}

func (o *FSCursorOps) CheckReleased() bool {
	return o == nil || o.cursor.CheckReleased() || o.base.CheckReleased()
}

func (o *FSCursorOps) GetName() string {
	return o.base.GetName()
}

func (o *FSCursorOps) GetIsDirectory() bool {
	return o.base.GetIsDirectory()
}

func (o *FSCursorOps) GetIsFile() bool {
	return o.base.GetIsFile()
}

func (o *FSCursorOps) GetIsSymlink() bool {
	return o.base.GetIsSymlink()
}

func (o *FSCursorOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	var out fs.FileMode
	err := o.readOps(ctx, func(ops unixfs.FSCursorOps) error {
		var err error
		out, err = ops.GetPermissions(ctx)
		return err
	})
	return out, err
}

func (o *FSCursorOps) SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error {
	return o.mutateCurrent(ctx, "set-permissions", func(ops unixfs.FSCursorOps) error {
		return ops.SetPermissions(ctx, permissions, ts)
	})
}

func (o *FSCursorOps) GetSize(ctx context.Context) (uint64, error) {
	var out uint64
	err := o.readOps(ctx, func(ops unixfs.FSCursorOps) error {
		var err error
		out, err = ops.GetSize(ctx)
		return err
	})
	return out, err
}

func (o *FSCursorOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	var out time.Time
	err := o.readOps(ctx, func(ops unixfs.FSCursorOps) error {
		var err error
		out, err = ops.GetModTimestamp(ctx)
		return err
	})
	return out, err
}

func (o *FSCursorOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	return o.mutateCurrent(ctx, "set-mod-timestamp", func(ops unixfs.FSCursorOps) error {
		return ops.SetModTimestamp(ctx, mtime)
	})
}

func (o *FSCursorOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	var out int64
	err := o.readOps(ctx, func(ops unixfs.FSCursorOps) error {
		var err error
		out, err = ops.ReadAt(ctx, offset, data)
		return err
	})
	return out, err
}

func (o *FSCursorOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	var out int64
	err := o.readOps(ctx, func(ops unixfs.FSCursorOps) error {
		var err error
		out, err = ops.GetOptimalWriteSize(ctx)
		return err
	})
	return out, err
}

func (o *FSCursorOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	return o.mutateCurrent(ctx, "write-at", func(ops unixfs.FSCursorOps) error {
		return ops.WriteAt(ctx, offset, data, ts)
	})
}

func (o *FSCursorOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	return o.mutateCurrent(ctx, "truncate", func(ops unixfs.FSCursorOps) error {
		return ops.Truncate(ctx, nsize, ts)
	})
}

func (o *FSCursorOps) Lookup(ctx context.Context, name string) (unixfs.FSCursor, error) {
	if o.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	if err := validateDirentName(name); err != nil {
		return nil, err
	}
	if alloc := o.cursor.lookupAllocation(); alloc != nil {
		return o.lookupWritableChild(ctx, alloc, name)
	}
	baseChild, err := o.base.Lookup(ctx, name)
	if err != nil {
		return nil, err
	}
	return o.cursor.child(name, baseChild), nil
}

func (o *FSCursorOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	return o.readOps(ctx, func(ops unixfs.FSCursorOps) error {
		return ops.ReaddirAll(ctx, skip, cb)
	})
}

func (o *FSCursorOps) Mknod(ctx context.Context, checkExist bool, names []string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	if err := validateDirentNames(names); err != nil {
		return err
	}
	return o.mutateChildren(ctx, "mknod", names, func(ops unixfs.FSCursorOps) error {
		return ops.Mknod(ctx, checkExist, names, nodeType, permissions, ts)
	})
}

func (o *FSCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, targetIsAbsolute bool, ts time.Time) error {
	if err := validateDirentName(name); err != nil {
		return err
	}
	return o.mutateChildren(ctx, "symlink", []string{name}, func(ops unixfs.FSCursorOps) error {
		return ops.Symlink(ctx, checkExist, name, target, targetIsAbsolute, ts)
	})
}

func (o *FSCursorOps) Readlink(ctx context.Context, name string) ([]string, bool, error) {
	var pathNodes []string
	var isAbsolute bool
	err := o.readOps(ctx, func(ops unixfs.FSCursorOps) error {
		var err error
		pathNodes, isAbsolute, err = ops.Readlink(ctx, name)
		return err
	})
	return pathNodes, isAbsolute, err
}

func (o *FSCursorOps) CopyTo(ctx context.Context, tgtDir unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	if err := validateDirentName(tgtName); err != nil {
		return true, err
	}
	tgt, ok := tgtDir.(*FSCursorOps)
	if !ok {
		return false, nil
	}
	return true, o.copyToLazyTarget(ctx, tgt, tgtName, false, ts)
}

func (o *FSCursorOps) CopyFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	if err := validateDirentName(name); err != nil {
		return true, err
	}
	src, ok := srcCursorOps.(*FSCursorOps)
	if !ok {
		return false, nil
	}
	return src.CopyTo(ctx, o, name, ts)
}

func (o *FSCursorOps) MoveTo(ctx context.Context, tgtCursorOps unixfs.FSCursorOps, tgtName string, ts time.Time) (bool, error) {
	if err := validateDirentName(tgtName); err != nil {
		return true, err
	}
	tgt, ok := tgtCursorOps.(*FSCursorOps)
	if !ok {
		return false, nil
	}
	if err := o.allocateLazyMove(ctx, tgt, tgtName); err != nil {
		return true, err
	}
	return false, nil
}

func (o *FSCursorOps) MoveFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (bool, error) {
	if err := validateDirentName(name); err != nil {
		return true, err
	}
	src, ok := srcCursorOps.(*FSCursorOps)
	if !ok {
		return false, nil
	}
	if err := src.allocateLazyMove(ctx, o, name); err != nil {
		return true, err
	}
	return false, nil
}

func (o *FSCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	if err := validateDirentNames(names); err != nil {
		return err
	}
	return o.mutateChildren(ctx, "remove", names, func(ops unixfs.FSCursorOps) error {
		return ops.Remove(ctx, names, ts)
	})
}

func (o *FSCursorOps) MknodWithContent(ctx context.Context, name string, nodeType unixfs.FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error {
	if err := validateDirentName(name); err != nil {
		return err
	}
	return o.mutateChildren(ctx, "mknod-with-content", []string{name}, func(ops unixfs.FSCursorOps) error {
		return ops.MknodWithContent(ctx, name, nodeType, dataLen, rdr, permissions, ts)
	})
}

func (o *FSCursorOps) readOps(ctx context.Context, cb func(ops unixfs.FSCursorOps) error) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if alloc := o.cursor.lookupAllocation(); alloc != nil {
		return o.withWritableOps(ctx, alloc, cb)
	}
	return cb(o.base)
}

func (o *FSCursorOps) mutateCurrent(ctx context.Context, operation string, cb func(ops unixfs.FSCursorOps) error) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	alloc, _, err := o.cursor.ensureWritable(ctx, operation, o.cursor.path)
	if err != nil {
		return err
	}
	return o.withWritableOps(ctx, alloc, cb)
}

func (o *FSCursorOps) mutateChildren(ctx context.Context, operation string, names []string, cb func(ops unixfs.FSCursorOps) error) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	var alloc *allocatedRoot
	for _, name := range names {
		childPath := append(append([]string(nil), o.cursor.path...), name)
		nextAlloc, _, err := o.cursor.ensureWritable(ctx, operation, childPath)
		if err != nil {
			return err
		}
		if alloc == nil {
			alloc = nextAlloc
		}
	}
	if alloc == nil {
		return cb(o.base)
	}
	return o.withWritableOps(ctx, alloc, cb)
}

func (o *FSCursorOps) withWritableOps(ctx context.Context, alloc *allocatedRoot, cb func(ops unixfs.FSCursorOps) error) error {
	handle, err := o.cursor.openWritableHandle(ctx, alloc)
	if err != nil {
		return err
	}
	defer handle.Release()
	return handle.AccessOps(ctx, func(_ unixfs.FSCursor, ops unixfs.FSCursorOps) error {
		return cb(ops)
	})
}

func (o *FSCursorOps) lookupWritableChild(ctx context.Context, alloc *allocatedRoot, name string) (unixfs.FSCursor, error) {
	handle, err := o.cursor.openWritableHandle(ctx, alloc)
	if err != nil {
		return nil, err
	}
	child, err := handle.Lookup(ctx, name)
	handle.Release()
	if err != nil {
		return nil, err
	}
	return unixfs.NewFSHandleCursor(child, true, nil), nil
}

func (o *FSCursorOps) copyToLazyTarget(ctx context.Context, tgt *FSCursorOps, tgtName string, move bool, ts time.Time) error {
	operation := "copy-to"
	srcAlloc, _, err := o.cursor.ensureWritable(ctx, operation, o.cursor.path)
	if err != nil {
		return err
	}
	tgtPath := append(append([]string(nil), tgt.cursor.path...), tgtName)
	tgtAlloc, _, err := tgt.cursor.ensureWritable(ctx, operation, tgtPath)
	if err != nil {
		return err
	}
	srcHandle, err := o.cursor.openWritableHandle(ctx, srcAlloc)
	if err != nil {
		return err
	}
	defer srcHandle.Release()
	tgtHandle, err := tgt.cursor.openWritableHandle(ctx, tgtAlloc)
	if err != nil {
		return err
	}
	defer tgtHandle.Release()
	return copyHandleToDir(ctx, srcHandle, tgtHandle, tgtName, ts)
}

func (o *FSCursorOps) allocateLazyMove(ctx context.Context, tgt *FSCursorOps, tgtName string) error {
	if _, _, err := o.cursor.ensureWritable(ctx, "move-to", o.cursor.path); err != nil {
		return err
	}
	tgtPath := append(append([]string(nil), tgt.cursor.path...), tgtName)
	_, _, err := tgt.cursor.ensureWritable(ctx, "move-to", tgtPath)
	return err
}

func copyHandleToDir(ctx context.Context, src *unixfs.FSHandle, tgtDir *unixfs.FSHandle, tgtName string, ts time.Time) error {
	nt, err := src.GetNodeType(ctx)
	if err != nil {
		return err
	}
	perms, err := src.GetPermissions(ctx)
	if err != nil {
		return err
	}
	switch {
	case nt.GetIsFile():
		size, err := src.GetSize(ctx)
		if err != nil {
			return err
		}
		const maxInt64 = uint64(1<<63 - 1)
		if size > maxInt64 {
			return unixfs_errors.ErrInvalidWrite
		}
		return tgtDir.MknodWithContent(ctx, tgtName, nt, int64(size), &fsHandleReader{ctx: ctx, handle: src, size: size}, perms, ts) //nolint:gosec
	case nt.GetIsDirectory():
		if err := tgtDir.Mknod(ctx, false, []string{tgtName}, nt, perms, ts); err != nil && err != unixfs_errors.ErrExist {
			return err
		}
		nextTgtDir, err := tgtDir.Lookup(ctx, tgtName)
		if err != nil {
			return err
		}
		defer nextTgtDir.Release()
		return src.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
			child, err := src.Lookup(ctx, ent.GetName())
			if err != nil {
				return err
			}
			defer child.Release()
			return copyHandleToDir(ctx, child, nextTgtDir, ent.GetName(), ts)
		})
	case nt.GetIsSymlink():
		target, isAbs, err := src.Readlink(ctx, "")
		if err != nil {
			return err
		}
		return tgtDir.Symlink(ctx, false, tgtName, target, isAbs, ts)
	default:
		return unixfs_errors.ErrNotFile
	}
}

type fsHandleReader struct {
	ctx    context.Context
	handle *unixfs.FSHandle
	offset int64
	size   uint64
}

func (r *fsHandleReader) Read(data []byte) (int, error) {
	if r.offset >= int64(r.size) { //nolint:gosec
		return 0, io.EOF
	}
	remaining := int64(r.size) - r.offset //nolint:gosec
	if int64(len(data)) > remaining {
		data = data[:remaining]
	}
	n, err := r.handle.ReadAt(r.ctx, r.offset, data)
	r.offset += n
	return int(n), err
}

func validateDirentNames(names []string) error {
	for _, name := range names {
		if err := validateDirentName(name); err != nil {
			return err
		}
	}
	return nil
}

func validateDirentName(name string) error {
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") {
		return fs.ErrInvalid
	}
	return nil
}

var _ unixfs.FSCursorOps = (*FSCursorOps)(nil)
