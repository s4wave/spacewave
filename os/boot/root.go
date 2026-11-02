//go:build linux

// Package boot runs the bounded immutable-root boot experiment. Bootstrap code,
// its runtime pages and the in-memory Volume stay outside the workload root.
package boot

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	volume_inmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// rootKey selects the single UnixFS object in this disposable World.
const rootKey = "os/root"

// Root retains the Volume, World snapshot and UnixFS handle until unmount.
type Root struct {
	// volume holds the physical blocks in bootstrap memory.
	volume *volume_inmem.KVTxInmem
	// engine owns the mutable World used to construct the image.
	engine *world_block.Engine
	// snapshot pins exactly the committed image revision.
	snapshot world.Tx
	// handle exposes only the pinned UnixFS revision without a writer.
	handle *unixfs.FSHandle
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

	// Construct the image with the same World operations ordinary UnixFS uses.
	ws := world.NewEngineWorldState(r.engine, true)
	if _, _, err := unixfs_world.FsInit(ctx, ws, r.volume.GetPeerID(), rootKey, unixfs_world.FSType_FSType_FS_NODE, nil, false, time.Now()); err != nil {
		return nil, err
	}
	fsc, _ := unixfs_world.NewFSCursorWithWriterContext(ctx, le, ws, rootKey, unixfs_world.FSType_FSType_FS_NODE, r.volume.GetPeerID())
	writable, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		return nil, err
	}
	defer writable.Release()
	if err := writeImageFile(ctx, writable, "workload", executable); err != nil {
		return nil, err
	}
	if err := writeImageFile(ctx, writable, "payload", newPayload()); err != nil {
		return nil, err
	}

	// Pin the committed revision before enabling kernel page caching. Later
	// World writes cannot change this transaction or the writerless cursor.
	r.snapshot, err = r.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	r.handle, err = unixfs.NewFSHandle(unixfs_world.NewFSCursorWithContext(ctx, le, r.snapshot, rootKey, unixfs_world.FSType_FSType_FS_NODE, nil, false))
	if err != nil {
		return nil, err
	}
	return r, nil
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
	}

	// Preserve either storage close error for the caller.
	var err error
	if r.engine != nil {
		err = r.engine.Close()
	}
	if r.volume != nil {
		err = errors.Join(err, r.volume.Close())
	}
	return err
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
