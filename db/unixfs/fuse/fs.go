//go:build linux

package fuse

import (
	"context"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	_ "bazil.org/fuse/fs/fstestutil"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/sirupsen/logrus"
)

const (
	// refBlockSize is the fake block size used for Block counts. The
	// filesystem has no block size, but unix expects one.
	refBlockSize = 512
	// nodeValidTime is how long the kernel caches attributes and entries of a
	// writable mount. The kernel may forget parts of the tree sooner under
	// memory pressure.
	nodeValidTime = time.Minute * 5
	// readOnlyValidTime is how long the kernel caches attributes and entries
	// of a read-only mount, whose tree never changes.
	readOnlyValidTime = time.Hour * 24
)

// RootFS mounts the root userspace filesystem resources.
type RootFS struct {
	// ctx is the root context
	ctx context.Context
	// ctxCancel is the context cancel
	ctxCancel context.CancelFunc
	// le is the logger
	le *logrus.Entry
	// rootPath is the path to mount the FUSE filesystem
	rootPath string
	// readOnly serves an unchanging tree through the kernel page cache.
	readOnly bool
	// root is the root inode
	root *Inode
	// conn is the fuse connection
	conn *fuse.Conn
	// server is the root filesystem server
	server *fs.Server
}

// MountOption is an additional mount option.
type MountOption = fuse.MountOption

// Mount builds a new RootFS FUSE instance.
//
// A writable mount bypasses the kernel page cache so each write reaches the
// World before FUSE reports it. A readOnly mount must serve a tree that never
// changes, such as one pinned revision: the kernel rejects writes, caches file
// pages, attributes and entries, and memory-maps files through its page cache.
func Mount(
	ctx context.Context,
	le *logrus.Entry,
	rootPath string,
	rootHandle *unixfs.FSHandle,
	verbose bool,
	readOnly bool,
	mountOpts []fuse.MountOption,
) (*RootFS, error) {
	// Build the root inode.
	rootFS := &RootFS{rootPath: rootPath, le: le, readOnly: readOnly}
	rootFS.ctx, rootFS.ctxCancel = context.WithCancel(ctx)
	rootFS.root = NewInode(rootFS, nil, rootHandle)

	// Mount the kernel filesystem.
	mountOpts = append([]MountOption{
		fuse.FSName("hydrafs"),
		fuse.Subtype("hydrafs"),
	}, mountOpts...)
	if readOnly {
		mountOpts = append(mountOpts, fuse.ReadOnly())
	}
	var err error
	rootFS.conn, err = fuse.Mount(
		rootPath,
		mountOpts...,
	)
	if err != nil {
		rootFS.ctxCancel()
		return nil, err
	}

	// Build the request server, logging requests when verbose.
	type stringable interface {
		String() string
	}
	srv := fs.New(rootFS.conn, &fs.Config{
		Debug: func(msg interface{}) {
			if verbose {
				sb, ok := msg.(stringable)
				if ok {
					le.Debug(sb.String())
				}
			}
		},
	})
	rootFS.server = srv
	return rootFS, nil
}

// Unmount tries to unmount the filesystem at dir.
func Unmount(path string) error {
	return fuse.Unmount(path)
}

// Serve runs the goroutine to respond to requests from FUSE.
func (r *RootFS) Serve() error {
	return r.server.Serve(r)
}

// GetConn returns the fuse connection
func (r *RootFS) GetConn() *fuse.Conn {
	return r.conn
}

// GetServer returns the root filesystem server
func (r *RootFS) GetServer() *fs.Server {
	return r.server
}

// GetReadOnly returns whether the mount is read-only.
func (r *RootFS) GetReadOnly() bool {
	return r.readOnly
}

// validTime returns how long the kernel may cache attributes and entries.
func (r *RootFS) validTime() time.Duration {
	if r.readOnly {
		return readOnlyValidTime
	}
	return nodeValidTime
}

// Root is called to obtain the Node for the file system root.
func (r *RootFS) Root() (fs.Node, error) {
	return r.root, nil
}

// logFilesystemError handles the filesystem error and logs it.
func (r *RootFS) logFilesystemError(err error) {
	// ignore context=canceled -> EINTR
	if err == context.Canceled {
		return
	}

	r.le.WithError(err).Warn("filesystem error")
}

// Close closes the FUSE instance.
func (r *RootFS) Close() {
	r.conn.Close()
	r.ctxCancel()
}

// _ is a type assertion
var _ fs.FS = (*RootFS)(nil)
