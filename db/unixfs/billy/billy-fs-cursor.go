package unixfs_billy

import (
	"context"
	"errors"
	"os"
	"path"
	"sync/atomic"

	"github.com/go-git/go-billy/v6"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// BillyFSCursor is an FSCursor backed by a Billy filesystem.
type BillyFSCursor struct {
	// released records whether this cursor has been released.
	released atomic.Bool
	// state is shared by cursors descended from the same root.
	state *billyFSState
	// path is the cursor's path in the Billy filesystem.
	path string
}

// NewBillyFSCursor constructs a FSCursor from a Billy filesystem at the given path.
// The path can be empty to build at the root of the filesystem.
func NewBillyFSCursor(bfs billy.Basic, path string) *BillyFSCursor {
	return NewVersionedBillyFSCursor(bfs, path, nil)
}

// NewVersionedBillyFSCursor builds a cursor with optional file versions.
// version reads metadata from the open file that supplies a versioned range.
// A nil version function leaves the file version unset.
func NewVersionedBillyFSCursor(bfs billy.Basic, path string, version func(billy.File) ([]byte, error)) *BillyFSCursor {
	// Choose the read lock based on the backend's concurrency contract.
	state := &billyFSState{bfs: bfs, native: isNativeOSFS(bfs), version: version}
	state.readMtx = &state.mtx
	if state.native {
		state.readMtx = state.mtx.RLocker()
	}

	// Share the backend and locks with every child cursor.
	return &BillyFSCursor{state: state, path: path}
}

// Clone opens an independent cursor at the same path on the shared filesystem.
func (c *BillyFSCursor) Clone() *BillyFSCursor {
	return &BillyFSCursor{state: c.state, path: c.path}
}

// CheckReleased checks if the fs cursor is currently released.
func (c *BillyFSCursor) CheckReleased() bool {
	return c.released.Load()
}

// GetProxyCursor returns a FSCursor to replace this one, if necessary.
// Return nil, nil if no redirection necessary (in most cases).
func (c *BillyFSCursor) GetProxyCursor(ctx context.Context) (unixfs.FSCursor, error) {
	return nil, nil
}

// AddChangeCb adds a change callback to detect when the cursor has changed.
// AddChangeCb is called only if GetProxyCursor returns nil, nil.
//
// cb must not block, and should be called when cursor changes / is released
// cb will be called immediately (same call tree) if already released.
func (c *BillyFSCursor) AddChangeCb(cb unixfs.FSCursorChangeCb) {
	if c.released.Load() {
		cb(&unixfs.FSCursorChange{Cursor: c, Released: true})
	}
}

// GetCursorOps returns the FSCursorOps for the FSCursor.
// Called after AddChangeCb and only if GetProxyCursor returns nil, nil.
// Returning nil, nil will be corrected to nil, ErrNotExist.
// Return nil, ErrReleased to indicate this FSCursor was released.
func (c *BillyFSCursor) GetCursorOps(ctx context.Context) (unixfs.FSCursorOps, error) {
	// Require a live Billy cursor before retrieving its operations.
	if c.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	// Read stable metadata even on a backend whose Stat accesses mutable files.
	c.state.readMtx.Lock()
	fi, err := billyLstat(c.state.bfs, c.path)
	c.state.readMtx.Unlock()
	if err != nil {
		if os.IsNotExist(err) {
			err = unixfs_errors.ErrNotExist
		}
		return nil, err
	}

	// Retain the metadata snapshot for subsequent operations on this cursor.
	return &BillyFSCursorOps{
		c:  c,
		fi: fi,
	}, nil
}

// Release releases the filesystem cursor.
func (c *BillyFSCursor) Release() {
	c.released.Store(true)
}

// buildChildPath builds a path to a child.
func (c *BillyFSCursor) buildChildPath(name string) (string, error) {
	// Empty and root names refer to the current cursor.
	if name == "" || name == "/" || name == "." {
		return c.path, nil
	}

	// Reject a child path that resolves to the current cursor.
	npath := path.Join(c.path, name)
	if npath == c.path {
		return "", errors.New("lookup with empty name not supported")
	}
	return npath, nil
}

// billyLstat reads metadata without following symlinks on backends with Lstat.
func billyLstat(bfs billy.Basic, fpath string) (os.FileInfo, error) {
	// Preserve a symlink's identity on filesystems that expose Lstat.
	if sl, ok := bfs.(billy.Symlink); ok {
		return sl.Lstat(fpath)
	}

	// Use the basic filesystem interface for backends without symlinks.
	return bfs.Stat(fpath)
}

// _ is a type assertion
var _ unixfs.FSCursor = (*BillyFSCursor)(nil)
