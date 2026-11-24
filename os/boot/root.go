//go:build linux

// Package boot runs the bounded immutable-root boot experiment. Bootstrap code,
// its runtime pages and the backing Volume stay outside the workload root.
package boot

import (
	"context"
	"errors"
	"io"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	volume_inmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	os_image "github.com/s4wave/spacewave/os/image"
	"github.com/sirupsen/logrus"
)

// Root serves one image's UnixFS tree read-only until unmount. An image is
// immutable: its root block fixes every byte, so the tree needs no snapshot.
type Root struct {
	// fs reads the tree through the image's transform, with no writer.
	fs *unixfs_block_fs.FS
	// handle exposes the tree's root directory.
	handle *unixfs.FSHandle
	// closeStorage closes the storage fs reads, after fs.
	closeStorage func() error
	// tools requires the retained tool image rather than the diskless probe.
	tools bool
}

// NewRoot writes executable into a disposable in-memory image under a fresh
// transform and serves it. It does not promise persistence.
func NewRoot(ctx context.Context, le *logrus.Entry, executable io.Reader) (_ *Root, retErr error) {
	// Open disposable storage, closed on failure.
	vol, err := volume_inmem.NewKVTxInmem(ctx, le, &volume_inmem.Config{})
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, vol.Close())
		}
	}()

	// Write the diskless probe without the retained image's tools.
	transform, err := os_image.NewTransform()
	if err != nil {
		return nil, err
	}
	cs, err := imageCursor(ctx, le, vol, &bucket.ObjectRef{TransformConf: transform})
	if err != nil {
		return nil, err
	}
	defer cs.Release()
	image, err := WriteImage(ctx, cs, executable, nil, nil)
	if err != nil {
		return nil, err
	}
	return serveImage(ctx, le, vol, image, vol.Close, false)
}

// serveImage serves image from store read-only. Root.Close calls closeStorage
// once fs releases, and serveImage calls it on failure.
func serveImage(ctx context.Context, le *logrus.Entry, store bucket.BucketOps, image *bucket.ObjectRef, closeStorage func() error, tools bool) (_ *Root, retErr error) {
	// Keep the tree's lifetime apart from request cancellation; each FUSE
	// operation still receives its own request context.
	ctx = context.WithoutCancel(ctx)
	r := &Root{closeStorage: closeStorage, tools: tools}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, r.Close())
		}
	}()

	// Read the tree through a cursor with no writer.
	cs, err := imageCursor(ctx, le, store, image)
	if err != nil {
		return nil, err
	}
	r.fs = unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, cs, nil)
	r.handle, err = unixfs.NewFSHandle(r.fs)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Handle borrows the image's root directory for a mount. The caller must stop
// every filesystem user and join the mount's server before closing Root.
func (r *Root) Handle() *unixfs.FSHandle {
	return r.handle
}

// Close releases the image after its workload and FUSE server have stopped.
func (r *Root) Close() error {
	// Release the whole block FS before the root handle: cached FUSE child
	// references may survive unmount without Forget.
	if r.fs != nil {
		r.fs.Release()
	}
	if r.handle != nil {
		r.handle.Release()
	}
	println("SPACEWAVE_OS_SNAPSHOT_RELEASED")

	// Close storage only after its readers.
	var err error
	if r.closeStorage != nil {
		err = r.closeStorage()
	}
	if err == nil {
		println("SPACEWAVE_OS_STORAGE_CLOSED")
	}
	return err
}

// VerifyImage streams every required image byte before the mount is ready.
// The expected executable comes from independent bootstrap storage, not UnixFS.
func (r *Root) VerifyImage(ctx context.Context, executable io.Reader) error {
	// Reject incomplete executable content before entering the kernel adapter.
	if err := verifyImageFile(ctx, r.handle, "workload", executable); err != nil {
		return err
	}

	// The later lazy mmap separately checks the same payload through Linux FUSE.
	if err := verifyImageFile(ctx, r.handle, "payload", newPayload()); err != nil {
		return err
	}

	// Every retained image must supply the complete fixed shell/tool layout.
	if r.tools {
		return verifyTools(ctx, r.handle)
	}
	return nil
}
