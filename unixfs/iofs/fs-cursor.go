package unixfs_iofs

import (
	"context"
	"io/fs"
	"path"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// FSCursor implements a FSCursor attached to an io/fs.FS.
type FSCursor struct {
	// isReleased indicates if this cursor is released.
	isReleased atomic.Bool
	// fs is the filesystem
	fs fs.FS
	// depth is the inode depth relative to root of fs
	// note: not always accurate, used as an estimate
	depth uint
	// name is the name of this node
	name string
	// parent is the parent FSCursor or nil if none
	parent *FSCursor
	// fsCursorOps is the filesystem ops object.
	fsCursorOps *FSCursorOps
	// path is the path to this cursor
	path []string
}

// NewFSCursor constructs a new FSCursor at the root of the fs.
func NewFSCursor(ifs fs.FS) (*FSCursor, error) {
	return newFSCursor(ifs, nil, "", nil)
}

// newFSCursor constructs a new FSCursor with details.
// expects fs.rmtx to be locked.
// fsTree can be nil to defer looking up from parent until later
// if fsTree is not nil, constructs the fsOps immediately
// btx can be nil
// returns nil if the parent was already released.
func newFSCursor(
	ifs fs.FS,
	parent *FSCursor,
	name string,
	path []string,
) (*FSCursor, error) {
	var depth uint
	if parent != nil {
		depth = parent.depth + 1
	}
	c := &FSCursor{
		fs:     ifs,
		depth:  depth,
		parent: parent,
		name:   name,
		path:   path,
	}
	var err error
	c.fsCursorOps, err = newFSCursorOps(c, ifs)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// CheckReleased checks if the fscursor is released without locking anything.
func (f *FSCursor) CheckReleased() bool {
	return f.isReleased.Load()
}

// GetPath resolves and returns the path to this cursor.
// Note: do not edit the returned slice!
func (f *FSCursor) GetPath() []string {
	if f.parent == nil {
		return nil
	}

	return f.path
}

// AddChangeCb adds a change callback to detect when the cursor has changed.
// This will be called only if GetProxyCursor returns nil, nil.
func (f *FSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	// we do not support changing the fs.FS (read only)
}

// GetProxyCursor returns a FSCursor to replace this one, if necessary.
func (f *FSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	return nil, nil
}

// GetFSCursorOps returns the interface implementing FSCursorOps.
// Called after AddChangeCb and only if GetProxyCursor returns nil, nil.
// Return nil, nil to indicate this position is null (nothing here).
// Return nil, ErrReleased to indicate this FSCursor was released.
func (f *FSCursor) GetFSCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	return f.fsCursorOps, nil
}

// buildChildCursor locks fs.rmtx and builds a child cursor with a name
func (f *FSCursor) buildChildCursor(name string, dirent fs.DirEntry) (unixfs.FSCursor, error) {
	childPath := make([]string, len(f.path)+1)
	copy(childPath, f.path)
	childPath[len(childPath)-1] = name
	cursor, err := newFSCursor(f.fs, f, name, childPath)
	if err != nil {
		return nil, err
	}
	return cursor, nil
}

// buildPathString builds the path to this FSCursor.
func (f *FSCursor) buildPathString() string {
	if len(f.path) == 0 {
		// root of the fs
		return "."
	}
	return path.Join(f.path...)
}

// Release releases the filesystem cursor.
// note: locks rmtx. must NOT be locked when calling
func (f *FSCursor) Release() {
	if f.isReleased.Swap(true) {
		return
	}
	f.fsCursorOps.release()
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*FSCursor)(nil))
