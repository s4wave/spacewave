package unixfs_billy

import (
	"context"
	"io"
	"io/fs"
	"os"
	"slices"
	"sync/atomic"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
)

// BillyFSCursorOps is an FSCursor ops implementation backed by a BillyFS.
type BillyFSCursorOps struct {
	released atomic.Bool
	c        *BillyFSCursor
	fi       os.FileInfo
}

// CheckReleased implements FSCursorOps.
func (o *BillyFSCursorOps) CheckReleased() bool {
	return o.released.Load() || o.c.released.Load()
}

// CopyFrom implements FSCursorOps.
func (o *BillyFSCursorOps) CopyFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (done bool, err error) {
	// not implemented
	return false, nil
}

// CopyTo implements FSCursorOps.
func (o *BillyFSCursorOps) CopyTo(ctx context.Context, tgtDir unixfs.FSCursorOps, tgtName string, ts time.Time) (done bool, err error) {
	// not implemented
	return false, nil
}

// GetIsDirectory implements FSCursorOps.
func (o *BillyFSCursorOps) GetIsDirectory() bool {
	return o.fi.IsDir()
}

// GetIsFile implements FSCursorOps.
func (o *BillyFSCursorOps) GetIsFile() bool {
	return o.fi.Mode().IsRegular()
}

// GetIsSymlink implements FSCursorOps.
func (o *BillyFSCursorOps) GetIsSymlink() bool {
	return o.fi.Mode()&os.ModeSymlink == os.ModeSymlink
}

// GetModTimestamp implements FSCursorOps.
func (o *BillyFSCursorOps) GetModTimestamp(ctx context.Context) (time.Time, error) {
	return o.fi.ModTime(), nil
}

// GetName implements FSCursorOps.
func (o *BillyFSCursorOps) GetName() string {
	return o.fi.Name()
}

// GetOptimalWriteSize implements FSCursorOps.
func (o *BillyFSCursorOps) GetOptimalWriteSize(ctx context.Context) (int64, error) {
	return 512, nil
}

// GetPermissions implements FSCursorOps.
func (o *BillyFSCursorOps) GetPermissions(ctx context.Context) (fs.FileMode, error) {
	return o.fi.Mode().Perm(), nil
}

// GetSize implements FSCursorOps.
func (o *BillyFSCursorOps) GetSize(ctx context.Context) (uint64, error) {
	return uint64(o.fi.Size()), nil //nolint:gosec
}

// Lookup implements FSCursorOps.
func (o *BillyFSCursorOps) Lookup(ctx context.Context, name string) (unixfs.FSCursor, error) {
	if o.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	if !o.fi.IsDir() {
		return nil, unixfs_errors.ErrNotDirectory
	}

	npath, err := o.c.buildChildPath(name)
	if err != nil {
		return nil, err
	}

	_, err = billyLstat(o.c.bfs, npath)
	if err != nil {
		if os.IsNotExist(err) {
			err = unixfs_errors.ErrNotExist
		}
		return nil, err
	}

	return NewBillyFSCursor(o.c.bfs, npath), nil
}

// Mknod implements FSCursorOps.
func (o *BillyFSCursorOps) Mknod(ctx context.Context, checkExist bool, names []string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.fi.IsDir() {
		return unixfs_errors.ErrNotDirectory
	}
	if len(names) == 0 {
		return nil
	}

	createDir := nodeType.GetIsDirectory()
	var dirFs billy.Dir
	if createDir {
		var ok bool
		dirFs, ok = o.c.bfs.(billy.Dir)
		if !ok {
			return billy.ErrNotSupported
		}
	} else if !nodeType.GetIsFile() {
		return billy.ErrNotSupported
	}

	childPaths := make([]string, len(names))
	for i, name := range names {
		npath, err := o.c.buildChildPath(name)
		if err != nil {
			return err
		}
		childPaths[i] = npath
	}
	slices.Sort(childPaths)
	childPaths = slices.Compact(childPaths)

	if checkExist {
		for _, childPath := range childPaths {
			_, err := o.c.bfs.Stat(childPath)
			if err == nil {
				return unixfs_errors.ErrExist
			}
			if !os.IsNotExist(err) {
				return err
			}
		}
	}

	for _, childPath := range childPaths {
		o.released.Store(true) // release the cursor just before filesystem modification
		if createDir {
			if err := dirFs.MkdirAll(childPath, permissions); err != nil {
				return err
			}
		} else {
			f, err := o.c.bfs.OpenFile(childPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, permissions)
			if err != nil {
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}

	return nil
}

// MoveFrom implements FSCursorOps.
func (o *BillyFSCursorOps) MoveFrom(ctx context.Context, name string, srcCursorOps unixfs.FSCursorOps, ts time.Time) (done bool, err error) {
	// not implemented
	return false, nil
}

// MoveTo implements FSCursorOps.
func (o *BillyFSCursorOps) MoveTo(ctx context.Context, tgtCursorOps unixfs.FSCursorOps, tgtName string, ts time.Time) (done bool, err error) {
	// not implemented
	return false, nil
}

// ReadAt implements FSCursorOps.
func (o *BillyFSCursorOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}

	file, err := o.c.bfs.Open(o.c.path)
	if err != nil {
		if os.IsNotExist(err) {
			err = unixfs_errors.ErrNotExist
		}
		return 0, err
	}

	n, err := file.ReadAt(data, offset)
	return int64(n), err
}

// ReaddirAll implements FSCursorOps.
func (o *BillyFSCursorOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	if !o.fi.IsDir() {
		return unixfs_errors.ErrNotDirectory
	}

	dirFs, ok := o.c.bfs.(billy.Dir)
	if !ok {
		return unixfs_errors.ErrNotDirectory
	}

	fis, err := dirFs.ReadDir(o.c.path)
	if err != nil {
		if os.IsNotExist(err) {
			err = unixfs_errors.ErrNotExist
		}
		return err
	}
	if cb == nil {
		return nil
	}

	for _, fi := range fis {
		dirent := unixfs_iofs.NewFSCursorDirent(fi)
		if err := cb(dirent); err != nil {
			return err
		}
	}

	return nil
}

// Readlink implements FSCursorOps.
func (o *BillyFSCursorOps) Readlink(ctx context.Context, name string) ([]string, bool, error) {
	if o.CheckReleased() {
		return nil, false, unixfs_errors.ErrReleased
	}

	symlinkFs, ok := o.c.bfs.(billy.Symlink)
	if !ok {
		return nil, false, billy.ErrNotSupported
	}

	fpath, err := o.c.buildChildPath(name)
	if err != nil {
		return nil, false, err
	}

	outPath, err := symlinkFs.Readlink(fpath)
	if err != nil {
		return nil, false, err
	}

	nodes, isAbsolute := unixfs.SplitPath(outPath)
	return nodes, isAbsolute, nil
}

// Remove implements FSCursorOps.
func (o *BillyFSCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	removePaths := make([]string, len(names))
	for i, name := range names {
		fpath, err := o.c.buildChildPath(name)
		if err != nil {
			return err
		}
		removePaths[i] = fpath
	}
	slices.Sort(removePaths)
	removePaths = slices.Compact(removePaths)

	for _, removePath := range removePaths {
		o.released.Store(true) // release the cursor just before filesystem modification
		err := o.c.bfs.Remove(removePath)
		if err != nil && !os.IsNotExist(err) && err != unixfs_errors.ErrNotExist {
			return err
		}
	}

	return nil
}

// SetModTimestamp implements FSCursorOps.
func (o *BillyFSCursorOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	changeFs, ok := o.c.bfs.(billy.Change)
	if !ok {
		return billy.ErrNotSupported
	}

	o.released.Store(true) // release the cursor just before filesystem modification
	return changeFs.Chtimes(o.c.path, mtime, mtime)
}

// SetPermissions implements FSCursorOps.
func (o *BillyFSCursorOps) SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	changeFs, ok := o.c.bfs.(billy.Change)
	if !ok {
		return billy.ErrNotSupported
	}

	newMode := o.fi.Mode().Type() | permissions.Perm()
	o.released.Store(true) // release the cursor just before filesystem modification
	return changeFs.Chmod(o.c.path, newMode)
}

// Symlink implements FSCursorOps.
func (o *BillyFSCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, targetIsAbsolute bool, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	symlinkFs, ok := o.c.bfs.(billy.Symlink)
	if !ok {
		return billy.ErrNotSupported
	}

	fpath, err := o.c.buildChildPath(name)
	if err != nil {
		return err
	}

	if checkExist {
		_, err := o.c.bfs.Stat(fpath)
		if err == nil {
			return unixfs_errors.ErrExist
		}
		if !os.IsNotExist(err) {
			return err
		}
	}

	o.released.Store(true) // release the cursor just before filesystem modification
	return symlinkFs.Symlink(unixfs.JoinPath(target, targetIsAbsolute), fpath)
}

// Truncate implements FSCursorOps.
func (o *BillyFSCursorOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.fi.Mode().IsRegular() {
		return unixfs_errors.ErrNotFile
	}

	f, err := o.c.bfs.OpenFile(o.c.path, os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsNotExist(err) {
			err = unixfs_errors.ErrNotExist
		}
		return err
	}

	o.released.Store(true)                           // release the cursor just before filesystem modification
	if err := f.Truncate(int64(nsize)); err != nil { //nolint:gosec
		_ = f.Close()
		return err
	}

	return f.Close()
}

// WriteAt implements FSCursorOps.
func (o *BillyFSCursorOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.fi.Mode().IsRegular() {
		return unixfs_errors.ErrNotFile
	}

	f, err := o.c.bfs.OpenFile(o.c.path, os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsNotExist(err) {
			err = unixfs_errors.ErrNotExist
		}
		return err
	}

	_, err = f.Seek(offset, io.SeekStart)
	if err != nil {
		_ = f.Close()
		return err
	}

	o.released.Store(true) // release the cursor just before filesystem modification
	for len(data) != 0 {
		n, err := f.Write(data)
		if err != nil {
			_ = f.Close()
			return err
		}
		if n >= len(data) {
			break
		}
		data = data[n:]
	}

	return f.Close()
}

// MknodWithContent creates a file entry and writes content atomically.
func (o *BillyFSCursorOps) MknodWithContent(ctx context.Context, name string, nodeType unixfs.FSCursorNodeType, dataLen int64, rdr io.Reader, permissions fs.FileMode, ts time.Time) error {
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.fi.IsDir() {
		return unixfs_errors.ErrNotDirectory
	}

	fpath, err := o.c.buildChildPath(name)
	if err != nil {
		return err
	}

	o.released.Store(true) // release the cursor just before filesystem modification
	f, err := o.c.bfs.OpenFile(fpath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, permissions)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, rdr)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// _ is a type assertion
var _ unixfs.FSCursorOps = ((*BillyFSCursorOps)(nil))
