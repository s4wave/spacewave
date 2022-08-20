package fuse

import (
	"context"

	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/sirupsen/logrus"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	_ "bazil.org/fuse/fs/fstestutil"
)

// refBlockSize is the fake block size used for Block counts.
// there is no "block size" in this fs implementation, but unix expects it
const refBlockSize = 512

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
func Mount(
	ctx context.Context,
	le *logrus.Entry,
	rootPath string,
	ufs *unixfs.FS,
	verbose bool,
	mountOpts []fuse.MountOption,
) (*RootFS, error) {
	rref, err := ufs.AddRootReference(ctx)
	if err != nil {
		return nil, err
	}

	rootFS := &RootFS{rootPath: rootPath, le: le}
	rootFS.ctx, rootFS.ctxCancel = context.WithCancel(ctx)
	root := NewInode(rootFS, nil, rref)
	rootFS.root = root

	mountOpts = append(mountOpts,
		fuse.FSName("hydrafs"),
		fuse.Subtype("hydrafs"),
	)

	rootFS.conn, err = fuse.Mount(
		rootPath,
		mountOpts...,
	)
	if err != nil {
		rootFS.ctxCancel()
		rref.Release()
		return nil, err
	}

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
var _ fs.FS = ((*RootFS)(nil))
