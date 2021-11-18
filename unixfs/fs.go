package unixfs

import (
	"context"
	"sync/atomic"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
)

// FS implements a common filesystem access interface on top of FSCursor.
type FS struct {
	// isReleased is a uint32 atomic int
	isReleased uint32
	// le is the root logger
	// note: le may be nil
	le *logrus.Entry
	// ctx is the root context for the fs tree
	ctx context.Context
	// ctxCancel cancels ctx
	ctxCancel context.CancelFunc
	// rootFSCursor is the root filesystem cursor.
	rootFSCursor FSCursor
	// rootPrefix is a prefix to apply when building rootInode
	rootPrefix []string

	// waitSema guards below fields and the structure of the inode tree
	waitSema *semaphore.Weighted
	// rootInode is the root inode reference.
	// nil until AddRootReference is called.
	rootInode *fsInode
}

// NewFS constructs a new fs with a root cursor.
// le can be nil
func NewFS(
	ctx context.Context,
	le *logrus.Entry,
	rootFSCursor FSCursor,
	rootPrefix []string,
) *FS {
	subCtx, subCtxCancel := context.WithCancel(ctx)
	return &FS{
		le:           le,
		ctx:          subCtx,
		ctxCancel:    subCtxCancel,
		rootFSCursor: rootFSCursor,
		rootPrefix:   rootPrefix,

		waitSema: semaphore.NewWeighted(1),
	}
}

// CheckReleased checks if released without locking anything.
func (f *FS) CheckReleased() bool {
	return atomic.LoadUint32(&f.isReleased) == 1
}

// GetRootFSCursor returns the root FS cursor
func (f *FS) GetRootFSCursor() FSCursor {
	return f.rootFSCursor
}

// AddRootReference adds a reference to the root inode.
func (f *FS) AddRootReference(ctx context.Context) (*FSHandle, error) {
	if ctx == nil {
		ctx = f.ctx
	}
	if err := f.waitSema.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer f.waitSema.Release(1)

	rootInode, err := f.resolveRootInode()
	if err != nil {
		return nil, err
	}
	return rootInode.addReference()
}

// resolveRootInode gets or builds the root inode, applying the path prefix if necessary.
// caller must hold waitSema
func (f *FS) resolveRootInode() (*fsInode, error) {
	if f.CheckReleased() {
		return nil, unixfs_errors.ErrReleased
	}

	if f.rootInode != nil {
		if f.rootInode.checkReleased() {
			f.rootInode = nil
		} else {
			return f.rootInode, nil
		}
	}

	// build new root inode
	rootInode := newFsInode(f, nil, "")

	// apply path prefix if necessary
	// create a inode tree that has not yet been resolved.
	for _, prefixDir := range f.rootPrefix {
		if len(prefixDir) == 0 {
			continue
		}
		nchild := newFsInode(f, rootInode, prefixDir)
		rootInode.children = []*fsInode{nchild}
		rootInode = nchild
	}

	f.rootInode = rootInode
	return rootInode, nil
}

// Release releases the filesystem.
func (f *FS) Release() {
	if f.CheckReleased() {
		return
	}

	f.ctxCancel()
	if err := f.waitSema.Acquire(context.Background(), 1); err == nil {
		defer f.waitSema.Release(1)
	}
	if f.rootInode != nil {
		f.rootInode.releaseWithChildrenLocked(nil)
		f.rootInode = nil
	}
	if atomic.SwapUint32(&f.isReleased, 1) == 0 {
		if f.rootFSCursor != nil {
			f.rootFSCursor.Release()
		}
	}
}
