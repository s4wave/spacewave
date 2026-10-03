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
	// Require a live directory cursor before looking up a child.
	if o.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}
	if !o.fi.IsDir() {
		return nil, unixfs_errors.ErrNotDirectory
	}

	// Resolve the child name within the Billy filesystem.
	npath, err := o.c.buildChildPath(name)
	if err != nil {
		return nil, err
	}

	// Check the child entry while holding the Billy filesystem lock.
	o.c.state.mtx.Lock()
	_, err = billyLstat(o.c.state.bfs, npath)
	o.c.state.mtx.Unlock()
	if err != nil {
		if os.IsNotExist(err) {
			err = unixfs_errors.ErrNotExist
		}
		return nil, err
	}

	return newBillyFSCursor(o.c.state, npath), nil
}

// Mknod implements FSCursorOps.
func (o *BillyFSCursorOps) Mknod(ctx context.Context, checkExist bool, names []string, nodeType unixfs.FSCursorNodeType, permissions fs.FileMode, ts time.Time) error {
	// Require a live directory cursor and names for node creation.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.fi.IsDir() {
		return unixfs_errors.ErrNotDirectory
	}
	if len(names) == 0 {
		return nil
	}

	// Select the Billy capability for the requested node type.
	createDir := nodeType.GetIsDirectory()
	var dirFs billy.Dir
	if createDir {
		var ok bool
		dirFs, ok = o.c.state.bfs.(billy.Dir)
		if !ok {
			return billy.ErrNotSupported
		}
	} else if !nodeType.GetIsFile() {
		return billy.ErrNotSupported
	}

	// Resolve and deduplicate the child paths before creating nodes.
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

	// Check for existing entries under the Billy filesystem lock.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()
	if checkExist {
		for _, childPath := range childPaths {
			_, err := o.c.state.bfs.Stat(childPath)
			if err == nil {
				return unixfs_errors.ErrExist
			}
			if !os.IsNotExist(err) {
				return err
			}
		}
	}

	// Create each requested node and invalidate the cursor before mutation.
	for _, childPath := range childPaths {
		o.released.Store(true) // release the cursor just before filesystem modification
		if createDir {
			if err := dirFs.MkdirAll(childPath, permissions); err != nil {
				return err
			}
		} else {
			f, err := o.c.state.bfs.OpenFile(childPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, permissions)
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

// MoveTo implements FSCursorOps. It renames the source entry into tgtCursorOps
// under tgtName via the backing billy.Basic.Rename when the target lives in the
// same billy filesystem. It returns done=false for a different filesystem so
// FSHandle.Rename can fall back; billy.Basic.Rename overwrites a file target.
func (o *BillyFSCursorOps) MoveTo(ctx context.Context, tgtCursorOps unixfs.FSCursorOps, tgtName string, ts time.Time) (done bool, err error) {
	// Require a live cursor and a target in the same Billy filesystem.
	if o.CheckReleased() {
		return false, unixfs_errors.ErrReleased
	}
	tgtOps, ok := tgtCursorOps.(*BillyFSCursorOps)
	if !ok || tgtOps.c.state != o.c.state {
		return false, nil
	}

	// Resolve the destination path within the target directory.
	newPath, err := tgtOps.c.buildChildPath(tgtName)
	if err != nil {
		return false, err
	}

	// Rename the Billy entry under the filesystem lock and invalidate the cursor.
	o.released.Store(true) // release the cursor just before filesystem modification
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()
	if err := o.c.state.bfs.Rename(o.c.path, newPath); err != nil {
		return false, err
	}
	return true, nil
}

// ReadAt implements FSCursorOps.
func (o *BillyFSCursorOps) ReadAt(ctx context.Context, offset int64, data []byte) (int64, error) {
	// Require a live cursor before reading file content.
	if o.CheckReleased() {
		return 0, unixfs_errors.ErrReleased
	}

	// Open the Billy file under the filesystem lock.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()
	file, err := o.c.state.bfs.Open(o.c.path)
	if err != nil {
		if os.IsNotExist(err) {
			err = unixfs_errors.ErrNotExist
		}
		return 0, err
	}

	// Read the requested file range through the Billy file.
	n, err := file.ReadAt(data, offset)
	return int64(n), err
}

// ReaddirAll implements FSCursorOps.
func (o *BillyFSCursorOps) ReaddirAll(ctx context.Context, skip uint64, cb func(ent unixfs.FSCursorDirent) error) error {
	// Require a live cursor before listing directory entries.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// Require a directory entry for the Billy listing.
	if !o.fi.IsDir() {
		return unixfs_errors.ErrNotDirectory
	}

	// Require the Billy directory capability for the listing.
	dirFs, ok := o.c.state.bfs.(billy.Dir)
	if !ok {
		return unixfs_errors.ErrNotDirectory
	}

	// Read the Billy directory entries under the filesystem lock.
	o.c.state.mtx.Lock()
	fis, err := dirFs.ReadDir(o.c.path)
	o.c.state.mtx.Unlock()
	if err != nil {
		if os.IsNotExist(err) {
			err = unixfs_errors.ErrNotExist
		}
		return err
	}
	if cb == nil {
		return nil
	}

	// Deliver each Billy directory entry to the callback.
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
	// Require a live cursor before reading a symbolic link.
	if o.CheckReleased() {
		return nil, false, unixfs_errors.ErrReleased
	}

	// Require the Billy symbolic link capability.
	symlinkFs, ok := o.c.state.bfs.(billy.Symlink)
	if !ok {
		return nil, false, billy.ErrNotSupported
	}

	// Resolve the symbolic link name within the Billy filesystem.
	fpath, err := o.c.buildChildPath(name)
	if err != nil {
		return nil, false, err
	}

	// Read the symbolic link target under the filesystem lock.
	o.c.state.mtx.Lock()
	outPath, err := symlinkFs.Readlink(fpath)
	o.c.state.mtx.Unlock()
	if err != nil {
		return nil, false, err
	}

	// Decode the symbolic link target into UnixFS path components.
	nodes, isAbsolute := unixfs.SplitPath(outPath)
	return nodes, isAbsolute, nil
}

// Remove implements FSCursorOps.
func (o *BillyFSCursorOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	// Require a live cursor before removing child entries.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// Resolve and deduplicate the child paths before removing entries.
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

	// Remove the Billy entries under the filesystem lock and invalidate the cursor.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()
	for _, removePath := range removePaths {
		o.released.Store(true) // release the cursor just before filesystem modification
		err := o.c.state.bfs.Remove(removePath)
		if err != nil && !os.IsNotExist(err) && err != unixfs_errors.ErrNotExist {
			return err
		}
	}

	return nil
}

// chtimesFS is the minimal billy capability SetModTimestamp requires. Asserting
// it directly, rather than the full billy.Change, lets backings that support
// only timestamp changes (memfs supports neither, osfs supports all) avoid a
// false ErrNotSupported that surfaces to the guest as EIO.
type chtimesFS interface {
	Chtimes(name string, atime, mtime time.Time) error
}

// SetModTimestamp implements FSCursorOps.
func (o *BillyFSCursorOps) SetModTimestamp(ctx context.Context, mtime time.Time) error {
	// Require a live cursor before changing its timestamp.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// Require the Billy timestamp capability.
	chtimesFs, ok := o.c.state.bfs.(chtimesFS)
	if !ok {
		return billy.ErrNotSupported
	}

	// Change the Billy entry timestamp under the filesystem lock and invalidate the cursor.
	o.released.Store(true) // release the cursor just before filesystem modification
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()
	return chtimesFs.Chtimes(o.c.path, mtime, mtime)
}

// SetPermissions implements FSCursorOps.
func (o *BillyFSCursorOps) SetPermissions(ctx context.Context, permissions fs.FileMode, ts time.Time) error {
	// Require a live cursor before changing its permissions.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// Assert only billy.Chmod, not the full billy.Change: memfs (the RAM
	// writable-root upper) implements Chmod but not Lchown/Chown/Chtimes, so a
	// billy.Change assertion would return ErrNotSupported and the guest chmod
	// would collapse to EIO. apt requires chmod on /var/lib/apt and the cache.
	chmodFs, ok := o.c.state.bfs.(billy.Chmod)
	if !ok {
		return billy.ErrNotSupported
	}

	// Change the Billy entry permissions under the filesystem lock and invalidate the cursor.
	newMode := o.fi.Mode().Type() | permissions.Perm()
	o.released.Store(true) // release the cursor just before filesystem modification
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()
	return chmodFs.Chmod(o.c.path, newMode)
}

// Symlink implements FSCursorOps.
func (o *BillyFSCursorOps) Symlink(ctx context.Context, checkExist bool, name string, target []string, targetIsAbsolute bool, ts time.Time) error {
	// Require a live cursor before creating a symbolic link.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// Require the Billy symbolic link capability for creation.
	symlinkFs, ok := o.c.state.bfs.(billy.Symlink)
	if !ok {
		return billy.ErrNotSupported
	}

	// Resolve the new symbolic link path within the Billy filesystem.
	fpath, err := o.c.buildChildPath(name)
	if err != nil {
		return err
	}

	// Check for an existing Billy entry under the filesystem lock.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()
	if checkExist {
		_, err := o.c.state.bfs.Stat(fpath)
		if err == nil {
			return unixfs_errors.ErrExist
		}
		if !os.IsNotExist(err) {
			return err
		}
	}

	// Create the symbolic link and invalidate the cursor before mutation.
	o.released.Store(true) // release the cursor just before filesystem modification
	return symlinkFs.Symlink(unixfs.JoinPath(target, targetIsAbsolute), fpath)
}

// Truncate implements FSCursorOps.
func (o *BillyFSCursorOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	// Require a live regular-file cursor before truncation.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.fi.Mode().IsRegular() {
		return unixfs_errors.ErrNotFile
	}

	// Open the Billy file for truncation under the filesystem lock.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()
	f, err := o.c.state.bfs.OpenFile(o.c.path, os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsNotExist(err) {
			err = unixfs_errors.ErrNotExist
		}
		return err
	}

	// Truncate the Billy file and invalidate the cursor before mutation.
	o.released.Store(true)                           // release the cursor just before filesystem modification
	if err := f.Truncate(int64(nsize)); err != nil { //nolint:gosec
		_ = f.Close()
		return err
	}

	return f.Close()
}

// WriteAt implements FSCursorOps.
func (o *BillyFSCursorOps) WriteAt(ctx context.Context, offset int64, data []byte, ts time.Time) error {
	// Require a live regular-file cursor before writing content.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.fi.Mode().IsRegular() {
		return unixfs_errors.ErrNotFile
	}

	// Open the Billy file for writing under the filesystem lock.
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()
	f, err := o.c.state.bfs.OpenFile(o.c.path, os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsNotExist(err) {
			err = unixfs_errors.ErrNotExist
		}
		return err
	}

	// Position the Billy file at the requested write offset.
	_, err = f.Seek(offset, io.SeekStart)
	if err != nil {
		_ = f.Close()
		return err
	}

	// Write the remaining content and invalidate the cursor before mutation.
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
	// Require a live directory cursor before creating a file with content.
	if o.CheckReleased() {
		return unixfs_errors.ErrReleased
	}
	if !o.fi.IsDir() {
		return unixfs_errors.ErrNotDirectory
	}

	// Resolve the new file path within the Billy filesystem.
	fpath, err := o.c.buildChildPath(name)
	if err != nil {
		return err
	}

	// Create the Billy file under the filesystem lock and invalidate the cursor.
	o.released.Store(true) // release the cursor just before filesystem modification
	o.c.state.mtx.Lock()
	defer o.c.state.mtx.Unlock()
	f, err := o.c.state.bfs.OpenFile(fpath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, permissions)
	if err != nil {
		return err
	}

	// Copy the supplied content and close the Billy file.
	_, err = io.Copy(f, rdr)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// _ is a type assertion
var _ unixfs.FSCursorOps = (*BillyFSCursorOps)(nil)
