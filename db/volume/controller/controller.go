package volume_controller

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
	volume "github.com/s4wave/spacewave/db/volume"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	"github.com/sirupsen/logrus"
)

// Controller implements a common volume controller.
//
// The controller manages a volume's lifecycle, including setup, teardown,
// garbage collection, and background tasks. The volume interface is implemented
// by many volume types, which then use the common volume controller.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// config is the volume controller config
	config *Config
	// bus is the controller bus
	bus bus.Bus
	// ctor is the constructor
	ctor volume.Constructor
	// volume contains the controlled volume
	// contains nil if the volume is not ready
	volume *ccontainer.CContainer[*volumeCtxPair]
	// controllerInfo contains the controller info
	controllerInfo *controller.Info

	// reconcilerMtx guards the desired reconciler key set.
	reconcilerMtx sync.Mutex
	// reconcilerKeys contains the desired running reconciler keys.
	reconcilerKeys map[bucket_store.BucketReconcilerPair]struct{}
	// reconcilers contains running reconciler routines keyed by bucket/reconciler pair.
	reconcilers *keyed.Keyed[bucket_store.BucketReconcilerPair, *runningReconciler]
	// bucketHandles contains open bucket handles
	// key: bucket id
	bucketHandles *keyed.KeyedRefCount[string, *bucketHandleTracker]
}

// volumeCtxPair is a volume and ctx pair.
type volumeCtxPair struct {
	vol volume.Volume
	ctx context.Context
}

// NewController constructs a new volume controller.
func NewController(
	le *logrus.Entry,
	config *Config,
	bus bus.Bus,
	info *controller.Info,
	ctor volume.Constructor,
) *Controller {
	if config == nil {
		config = &Config{}
	}

	ctrl := &Controller{
		le:             le,
		config:         config,
		bus:            bus,
		controllerInfo: info,
		ctor:           ctor,

		volume:         ccontainer.NewCContainer[*volumeCtxPair](nil),
		reconcilerKeys: make(map[bucket_store.BucketReconcilerPair]struct{}),
	}
	ctrl.reconcilers = keyed.NewKeyed(ctrl.newRunningReconciler)
	ctrl.bucketHandles = keyed.NewKeyedRefCount(ctrl.newBucketHandleTracker)
	return ctrl
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	volCtx, volCtxCancel := context.WithCancel(ctx)
	defer volCtxCancel()

	// Construct the volume.
	v, err := c.ctor(volCtx, c.le)
	if err != nil {
		return err
	}
	defer v.Close()

	le := c.le.WithField("peer-id", v.GetPeerID().String())
	le.Debug("volume constructed, initializing")
	errCh := make(chan error, 1)
	pushErr := func(err error) {
		if err == nil {
			return
		}
		select {
		case errCh <- err:
		default:
		}
	}
	go func() {
		if err := v.Execute(volCtx); err != nil {
			pushErr(err)
		}
	}()

	// load active bucket reconcilers
	if !c.config.GetDisableReconcilerQueues() {
		if err := c.wakeFilledReconcilerQueues(ctx, v); err != nil {
			le.WithError(err).Warn("unable to list filled bucket reconciler queues")
		}
	}

	// check the cache mode & wrap the volume if necessary
	if blockStoreID := c.config.GetBlockStoreId(); blockStoreID != "" {
		blkStore, _, blkStoreRef, err := block_store.ExLookupFirstBlockStore(ctx, c.bus, blockStoreID, false, func() {
			if ctx.Err() == nil {
				pushErr(errors.New("block store released"))
			}
		})
		if err != nil {
			return err
		}
		defer blkStoreRef.Release()

		overlayMode := c.config.GetBlockStoreOverlayMode()
		if overlayMode == block.OverlayMode_LOWER_ONLY {
			v = volume.NewVolumeBlockStore(v, blkStore)
		} else {
			writebackTimeoutDur, err := c.config.ParseBlockStoreWritebackTimeoutDur()
			if err != nil {
				le.WithError(err).Warnf(
					"write back timeout dur is invalid, using 30s: %v",
					c.config.GetBlockStoreWritebackTimeoutDur(),
				)
				writebackTimeoutDur = time.Second * 30
			}
			writebackPutOpts := c.config.GetBlockStoreWritebackPutOpts()
			v = volume.NewVolumeBlockStore(
				v,
				block.NewOverlay(
					ctx,
					v,
					blkStore,
					overlayMode,
					writebackTimeoutDur,
					writebackPutOpts,
				))
		}
		le.Debugf("wrapped volume with block store %s mode %s", blockStoreID, overlayMode.String())
	}

	// load the peer to the bus
	if !c.config.GetDisablePeer() {
		peerWithPriv, err := v.GetPeer(ctx, true)
		if err != nil {
			return err
		}

		peerCtrl := peer_controller.NewController(le, peerWithPriv)
		peerCtrlRel, err := c.bus.AddController(ctx, peerCtrl, nil)
		if err != nil {
			le.WithError(err).Warn("failed to mount the peer controller")
		} else {
			defer peerCtrlRel()
		}
	}

	le.WithField("volume-id", v.GetID()).Debug("volume ready")
	c.volume.SetValue(&volumeCtxPair{
		ctx: volCtx,
		vol: v,
	})
	c.reconcilers.SetContext(ctx, true)
	c.bucketHandles.SetContext(ctx, true)

	// Start GC sweep goroutine.
	go func() {
		if err := c.runGCSweep(volCtx); err != nil {
			pushErr(err)
		}
	}()

	select {
	case <-ctx.Done():
		err = ctx.Err()
	case err = <-errCh:
	}

	c.reconcilerMtx.Lock()
	c.reconcilerKeys = make(map[bucket_store.BucketReconcilerPair]struct{})
	c.reconcilerMtx.Unlock()
	c.reconcilers.SetContext(nil, false)
	c.reconcilers.SyncKeys(nil, false)
	c.bucketHandles.SetContext(nil, false)
	return err
}

// restartBucketHandle resets a bucket handle for a particular bucket id
//
// if conf is set, attempts to use instead of fetching it from the volume.
// if updating the handle was successful, returns the updated handle.
func (c *Controller) restartBucketHandle(bucketID string, conf *bucket.Config) *bucketHandle {
	if tracker, _ := c.bucketHandles.GetKey(bucketID); tracker != nil {
		return tracker.updateBucketConfig(conf)
	}
	_, _ = c.bucketHandles.RestartRoutine(bucketID)
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case volume.LookupVolume:
		return directive.R(c.resolveLookupVolume(ctx, di, d))
	case block_store.LookupBlockStore:
		return directive.R(c.resolveLookupBlockStore(ctx, di, d))
	case bucket.ApplyBucketConfig:
		return directive.R(c.resolveApplyBucketConf(ctx, di, d))
	case bucket.BuildBucketAPI:
		return directive.R(c.resolveBuildBucketAPI(ctx, di, d))
	case volume.ListBuckets:
		return directive.R(c.resolveListBuckets(ctx, di, d))
	case volume.BuildObjectStoreAPI:
		return directive.R(c.resolveBuildObjectStoreAPI(ctx, di, d))
	}

	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return c.controllerInfo
}

// GetVolume returns the controlled volume.
// This may wait for the volume to be ready.
func (c *Controller) GetVolume(ctx context.Context) (volume.Volume, error) {
	rv, err := c.volume.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return rv.vol, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ volume.Controller = ((*Controller)(nil))
