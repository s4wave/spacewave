package unixfs_billy

import (
	"context"
	"errors"
	"os"
	"path"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/go-git/go-billy/v6"
)

// BillyFSCursor is an FSCursor implementation backed by a BillyFS.
type BillyFSCursor struct {
	released atomic.Bool
	bfs      billy.Basic
	path     string
}

// NewBillyFSCursor constructs a FSCursor from a BillyFS at the given path.
// The path can be empty to build at the root of the fs.
func NewBillyFSCursor(bfs billy.Basic, path string) *BillyFSCursor {
	return &BillyFSCursor{bfs: bfs, path: path}
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
// This will be called only if GetProxyCursor returns nil, nil.
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
	if c.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	fi, err := billyLstat(c.bfs, c.path)
	if err != nil {
		if os.IsNotExist(err) {
			err = unixfs_errors.ErrNotExist
		}
		return nil, err
	}

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
	if name == "" || name == "/" || name == "." {
		return c.path, nil
	}

	npath := path.Join(c.path, name)
	if npath == c.path {
		return "", errors.New("lookup with empty name not supported")
	}
	return npath, nil
}

// billyLstat uses Lstat if available, otherwise falls back to Stat.
// Lstat does not follow symlinks, allowing symlink nodes to be resolved.
func billyLstat(bfs billy.Basic, fpath string) (os.FileInfo, error) {
	if sl, ok := bfs.(billy.Symlink); ok {
		return sl.Lstat(fpath)
	}
	return bfs.Stat(fpath)
}

// _ is a type assertion
var _ unixfs.FSCursor = ((*BillyFSCursor)(nil))
