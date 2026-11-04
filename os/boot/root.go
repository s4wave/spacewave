//go:build linux

// Package boot runs the bounded immutable-root boot experiment. Bootstrap code,
// its runtime pages and the backing Volume stay outside the workload root.
package boot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/volume"
	volume_inmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// rootKey selects the single UnixFS object in this disposable World.
const rootKey = "os/root"

// Root retains physical storage, a World snapshot and UnixFS handle until unmount.
type Root struct {
	// volume holds writable seed/baseline blocks outside the served tree.
	volume volume.Volume
	// volumeID is the retained serving identity read from the stored key.
	volumeID string
	// head is the exact selected serving revision.
	head *bucket.ObjectRef
	// publisher publishes the completed image through its tracked bucket.
	publisher block.AtomicPublisher
	// releaseStorage closes retained storage after readers, or joins the seed
	// controller which owns and closes its writable Volume.
	releaseStorage func() error
	// engine owns the mutable World used to construct the image.
	engine *world_block.Engine
	// snapshot pins exactly the committed image revision.
	snapshot world.Tx
	// handle exposes only the pinned UnixFS revision without a writer.
	handle *unixfs.FSHandle
	// tools requires the retained tool image rather than the diskless probe.
	tools bool
}

// NewRoot builds a disposable real Volume and World, streams executable into
// UnixFS, then pins a read transaction. It does not promise persistence.
func NewRoot(ctx context.Context, le *logrus.Entry, executable io.Reader) (_ *Root, retErr error) {
	// Release partially built storage on failure.
	r := &Root{}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, r.Close())
		}
	}()

	// Assemble the ordinary Volume and World directly, without a testbed or bus.
	var err error
	r.volume, err = volume_inmem.NewKVTxInmem(ctx, le, &volume_inmem.Config{})
	if err != nil {
		return nil, err
	}
	cursor := bucket_lookup.NewCursor(ctx, nil, le, nil, r.volume, nil, nil, &bucket.BucketOpArgs{}, nil)
	r.engine, err = world_block.NewEngine(ctx, le, cursor, world.NewSchema(unixfs_world.UnixfsTypes).LookupOp, nil, false)
	if err != nil {
		return nil, err
	}

	// Construct and pin the diskless probe without the retained image's tools.
	if err := r.buildImage(ctx, le, executable, nil); err != nil {
		return nil, err
	}
	if err := r.pin(ctx, le); err != nil {
		return nil, err
	}
	return r, nil
}

// buildImage streams the probe and optional tools through ordinary World operations.
func (r *Root) buildImage(ctx context.Context, le *logrus.Entry, executable, busybox io.Reader) error {
	// Construct the image with the same World operations ordinary UnixFS uses.
	ws := world.NewEngineWorldState(r.engine, true)
	if _, _, err := unixfs_world.FsInit(ctx, ws, r.volume.GetPeerID(), rootKey, unixfs_world.FSType_FSType_FS_NODE, nil, false, time.Now()); err != nil {
		return err
	}
	fsc, _ := unixfs_world.NewFSCursorWithWriterContext(ctx, le, ws, rootKey, unixfs_world.FSType_FSType_FS_NODE, r.volume.GetPeerID())
	writable, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		return err
	}
	defer writable.Release()
	if err := writeImageFile(ctx, writable, "workload", executable); err != nil {
		return err
	}
	if err := writeImageFile(ctx, writable, "payload", newPayload()); err != nil {
		return err
	}

	// Retained images add tools; the diskless probe keeps its original two files.
	if busybox != nil {
		return buildTools(ctx, writable, busybox)
	}
	return nil
}

// pin retains one immutable World revision for kernel page caching.
func (r *Root) pin(ctx context.Context, le *logrus.Entry) error {
	// Later World writes cannot change this transaction or writerless cursor.
	var err error
	r.snapshot, err = r.engine.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	return r.openHandle(ctx, le)
}

// openHandle borrows the immutable transaction without tying its lifetime to
// request cancellation. Each operation still receives its own request context.
func (r *Root) openHandle(ctx context.Context, le *logrus.Entry) error {
	var err error
	r.handle, err = unixfs.NewFSHandle(unixfs_world.NewFSCursorWithContext(context.WithoutCancel(ctx), le, r.snapshot, rootKey, unixfs_world.FSType_FSType_FS_NODE, nil, false))
	if err != nil {
		return err
	}
	return nil
}

// Handle borrows the pinned UnixFS root for a mount. The caller must stop every
// filesystem user and join the mount's server before closing Root.
func (r *Root) Handle() *unixfs.FSHandle {
	return r.handle
}

// Close releases the image after its workload and FUSE server have stopped.
func (r *Root) Close() error {
	// Drop readers before closing the World and its physical store.
	if r.handle != nil {
		r.handle.Release()
	}
	if r.snapshot != nil {
		r.snapshot.Discard()
		println("SPACEWAVE_OS_SNAPSHOT_RELEASED")
	}

	// Preserve either storage close error for the caller.
	var err error
	if r.engine != nil {
		err = r.engine.Close()
	}
	if r.releaseStorage != nil {
		err = errors.Join(err, r.releaseStorage())
	}
	if r.releaseStorage == nil && r.volume != nil {
		err = errors.Join(err, r.volume.Close())
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

// verifyImageFile reads complete regular executable content with bounded memory.
// A nil expected stream checks readability and length against immutable metadata.
func verifyImageFile(ctx context.Context, root *unixfs.FSHandle, name string, expected io.Reader) (retErr error) {
	// Borrow an ordinary io/fs reader on the frozen World.
	file, err := unixfs_iofs.NewFS(ctx, root).Open(name)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Mode() != 0o755 || info.Size() == 0 {
		return errors.New("invalid image executable: " + name)
	}

	// Hash the entire stored stream without collecting its contents.
	actualHash := sha256.New()
	actualBytes, err := io.Copy(actualHash, file)
	if err != nil {
		return err
	}
	if actualBytes != info.Size() {
		return errors.New("incomplete image file: " + name)
	}

	// Independent probe inputs additionally prove the complete expected content.
	if expected != nil {
		expectedHash := sha256.New()
		expectedBytes, err := io.Copy(expectedHash, expected)
		if err != nil {
			return err
		}
		if actualBytes != expectedBytes || !bytes.Equal(actualHash.Sum(nil), expectedHash.Sum(nil)) {
			return errors.New("incomplete or changed image file: " + name)
		}
	}

	// Record the complete immutable bytes, including tools on every runtime reopen.
	println("SPACEWAVE_OS_CONTENT file=" + name + " bytes=" + strconv.FormatInt(actualBytes, 10) +
		" sha256=" + hex.EncodeToString(actualHash.Sum(nil)))
	return nil
}

// writeImageFile streams one bootstrap input into the committed UnixFS tree.
func writeImageFile(ctx context.Context, root *unixfs.FSHandle, name string, src io.Reader) error {
	// Create and retain the file handle until its writes are committed.
	ts := time.Now()
	if err := root.Mknod(ctx, true, []string{name}, unixfs.NewFSCursorNodeType_File(), 0o755, ts); err != nil {
		return err
	}
	file, err := root.Lookup(ctx, name)
	if err != nil {
		return err
	}
	defer file.Release()

	// Bound import memory independently of executable size.
	buf := make([]byte, 1<<20)
	var offset int64
	for {
		n, err := src.Read(buf)
		if n != 0 {
			if err := file.WriteAt(ctx, offset, buf[:n], ts); err != nil {
				return err
			}
			offset += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return file.Sync(ctx)
}
