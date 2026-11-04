//go:build linux

package boot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	core_storage "github.com/s4wave/spacewave/db/core/storage"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

const (
	// dataBucket scopes the writable filesystem's blocks and retention.
	dataBucket = "os-data"
	// dataHeadStore holds the engine controller's generated HeadState.
	dataHeadStore = "os-data"
	// dataHeadKey matches the engine controller's default head record.
	dataHeadKey = "world-head"
	// dataRootKey selects the writable UnixFS object, separate from the image.
	dataRootKey = "os/data"
)

// Data owns a writable UnixFS, its controllers and its physical s4db Volume.
// Normal FSHandle.Sync waits for coordinated durable block/head/retention
// publication. Controller failure may stop filesystem operations, but cannot
// physically close storage under users. The caller must contain users and join
// its FUSE server before Close; Wait reports when containment is needed.
type Data struct {
	// volume stays open until handles and controllers have been released.
	volume *volume_s4db.Volume
	// bus owns and joins the normal Volume, schema and World controllers.
	bus bus.Bus
	// engineController retains Close's cached error even after a failed bus removal.
	engineController *world_block_engine.Controller
	// handle retains the writer-backed UnixFS cursor until ordered shutdown.
	handle *unixfs.FSHandle
	// releaseBucket retains the bucket used to verify publication capabilities.
	releaseBucket func()
	// stopped reports either storage controller's terminal result.
	stopped chan error
	// closeOnce serializes cleanup and publishes closeErr to repeated callers.
	closeOnce sync.Once
	// closeErr preserves every shutdown failure, including physical Close.
	closeErr error
}

// CreateData exclusively creates a fresh disposable data directory and initializes
// its filesystem through normal World operations. A failed creation leaves the
// partial store for inspection. It never replaces an existing directory.
func CreateData(ctx context.Context, le *logrus.Entry, dir string) (*Data, error) {
	// Claim an absent fixture before any storage operation can create data.
	if err := os.Mkdir(dir, 0o700); err != nil {
		return nil, err
	}

	return openData(ctx, le, dir, true)
}

// OpenData opens an existing, supported data filesystem. Read-only inspection
// rejects unsupported formats, corrupt boot records and missing roots before the
// writable Volume constructor can create or migrate anything. File descendants
// are validated on access rather than scanned at boot.
// The disposable fixture has one boot process and must not be replaced during open.
func OpenData(ctx context.Context, le *logrus.Entry, dir string) (*Data, error) {
	// Preserve unsupported storage before invoking a constructor with write access.
	if err := inspectData(ctx, le, filepath.Join(dir, "data.s4wave")); err != nil {
		return nil, err
	}

	return openData(ctx, le, dir, false)
}

// openData composes native storage, a tracked bucket and a persisted World engine.
func openData(ctx context.Context, le *logrus.Entry, dir string, create bool) (_ *Data, retErr error) {
	// Keep physical Close solely in Data, including partially initialized opens.
	d := &Data{stopped: make(chan error, 2)}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, d.Close())
		}
	}()
	var err error
	d.volume, err = volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{
		Path: filepath.Join(dir, "data.s4wave"), SingleProcess: true,
		NoGenerateKey: !create, NoWriteKey: !create,
	})
	if err != nil {
		return nil, err
	}

	// Separate retained controllers from constructor/request cancellation. The bus
	// joins them on Close; their callbacks also interrupt a failed startup wait.
	lifeCtx := context.WithoutCancel(ctx)
	readyCtx, failStartup := context.WithCancelCause(ctx)
	defer failStartup(nil)
	b, resolver, err := controllerbus_core.NewCoreBus(lifeCtx, le)
	if err != nil {
		return nil, err
	}
	d.bus = b

	// Install the normal local bucket lookup stack without network controllers.
	core_storage.AddFactories(b, resolver)
	if _, err := b.AddController(lifeCtx, node_controller.NewController(&node_controller.Config{}, le, b), nil); err != nil {
		return nil, err
	}

	// The Volume controller may stop independently, but borrows physical storage.
	vol := &borrowedNativeVolume{Volume: d.volume}
	vc := volume_controller.NewController(le,
		&volume_controller.Config{DisablePeer: true, GcIntervalDur: "1h"}, d.bus,
		controller.NewInfo(volume_s4db.ControllerID, volume_s4db.Version, "OS data"),
		func(context.Context, *logrus.Entry) (volume.Volume, error) { return vol, nil })
	if _, err := d.bus.AddController(lifeCtx, vc, func(err error) {
		err = errors.Join(errors.New("data Volume controller stopped"), err)
		failStartup(err)
		d.stopped <- err
	}); err != nil {
		return nil, err
	}
	if _, err := vc.GetVolume(readyCtx); err != nil {
		return nil, errors.Join(err, context.Cause(readyCtx))
	}

	// Only explicit fixture creation may add bucket metadata.
	if create {
		if _, _, _, err := vol.ApplyBucketConfig(ctx, &bucket.Config{Id: dataBucket, Rev: 1}); err != nil {
			return nil, err
		}
	}

	// Check publication and retention on the exact tracked bucket the engine uses.
	bkt, release, err := bucket_lookup.StartBucketRWOperation(readyCtx, d.bus,
		&bucket.BucketOpArgs{BucketId: dataBucket, VolumeId: vol.GetID()})
	if err != nil {
		return nil, err
	}
	d.releaseBucket = release
	publisher, ok := bkt.(block.AtomicPublisher)
	if !ok || !publisher.SupportsAtomicPublication() || !block.SupportsRootRetention(bkt) ||
		publisher.AtomicPublicationVolumeID() != vol.GetID() {
		return nil, errors.New("data requires tracked atomic publication in its native Volume")
	}

	// An uncoordinated engine would defer durability beyond normal file Sync.
	capability, err := vol.Capability(ctx, coord.Scope{VolumeID: vol.GetID(), ObjectStoreID: dataHeadStore})
	if err != nil || capability == nil || !capability.Supported || !capability.Generations {
		return nil, errors.Join(errors.New("data requires durable coordinated generations"), err)
	}

	// Resolve real UnixFS operations on the same bus as the persisted engine.
	conf := world_block_engine.NewConfig("os-data", vol.GetID(), dataBucket, dataHeadStore, nil, nil, false)
	if _, err := d.bus.AddController(lifeCtx,
		world.NewLookupOpController("os-data-ops", conf.GetEngineId(), world.NewSchema(unixfs_world.UnixfsTypes).LookupOp), nil); err != nil {
		return nil, err
	}
	ec, err := world_block_engine.NewController(le, d.bus, conf, nil)
	if err != nil {
		return nil, err
	}
	d.engineController = ec
	if _, err := d.bus.AddController(lifeCtx, ec, func(err error) {
		err = errors.Join(errors.New("data World controller stopped"), err)
		failStartup(err)
		d.stopped <- err
	}); err != nil {
		return nil, errors.Join(err, ec.Close())
	}
	engine, err := ec.GetWorldEngine(readyCtx)
	if err != nil {
		return nil, errors.Join(err, context.Cause(readyCtx))
	}

	// Initialize only a freshly claimed fixture; reopen follows the persisted
	// generated head without a recovery flag, alternate head or reset operation.
	ws := world.NewEngineWorldState(engine, true)
	if create {
		if _, _, err := unixfs_world.FsInit(ctx, ws, vol.GetPeerID(), dataRootKey,
			unixfs_world.FSType_FSType_FS_NODE, nil, false, time.Now()); err != nil {
			return nil, err
		}
	}
	cursor, _ := unixfs_world.NewFSCursorWithWriterContext(lifeCtx, le, ws, dataRootKey,
		unixfs_world.FSType_FSType_FS_NODE, vol.GetPeerID())
	d.handle, err = unixfs.NewFSHandle(cursor)
	if err != nil {
		cursor.Release()
		return nil, err
	}
	return d, nil
}

// Handle borrows the writable root until Close. Release every child handle and
// stop filesystem users and servers before closing Data.
func (d *Data) Handle() *unixfs.FSHandle {
	return d.handle
}

// Wait reports a controller stop requiring the caller to contain filesystem
// users. It does not close storage; Close follows only after those users stop.
func (d *Data) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-d.stopped:
		return err
	}
}

// Close flushes the tree, releases cursors, joins controllers, then physically
// closes storage. The caller has already stopped all filesystem users.
func (d *Data) Close() error {
	d.closeOnce.Do(func() {
		// Release every filesystem cursor before controller cleanup.
		if d.handle != nil {
			d.closeErr = d.handle.Sync(context.Background())
			d.handle.Release()
		}
		if d.releaseBucket != nil {
			d.releaseBucket()
		}

		// Join controller cleanup before the sole physical Close, preserving errors.
		if d.bus != nil {
			d.closeErr = errors.Join(d.closeErr, d.bus.Close())
		}
		if d.engineController != nil {
			d.closeErr = errors.Join(d.closeErr, d.engineController.Close())
		}
		if d.volume != nil {
			d.closeErr = errors.Join(d.closeErr, d.volume.Close())
		}
	})
	return d.closeErr
}
