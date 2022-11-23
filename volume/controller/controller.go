package volume_controller

import (
	"context"
	"sync"

	peer_controller "github.com/aperturerobotics/bifrost/peer/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_store "github.com/aperturerobotics/hydra/bucket/store"
	volume "github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/util/ccontainer"
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
	// volumeCh contains the controlled volume
	// contains nil if the volume is not ready
	volume *ccontainer.CContainer[*volumeCtxPair]
	// controllerInfo contains the controller info
	controllerInfo *controller.Info

	// mtx guards below fields
	mtx sync.Mutex
	// reconcilers contains running reconciler instances.
	reconcilers map[bucket_store.BucketReconcilerPair]*runningReconciler
	// bucketHandles contains open bucket handles
	// key: bucket id
	bucketHandles map[string]*bucketHandle
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

	return &Controller{
		le:             le,
		config:         config,
		bus:            bus,
		controllerInfo: info,
		ctor:           ctor,

		volume:        ccontainer.NewCContainer[*volumeCtxPair](nil),
		reconcilers:   make(map[bucket_store.BucketReconcilerPair]*runningReconciler),
		bucketHandles: make(map[string]*bucketHandle),
	}
}

// Execute executes the given controller.
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

	le := c.le.WithField("peer-id", v.GetPeerID().Pretty())
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

	le.Info("volume ready")
	c.volume.SetValue(&volumeCtxPair{
		ctx: volCtx,
		vol: v,
	})

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

	select {
	case <-ctx.Done():
		err = ctx.Err()
	case err = <-errCh:
	}

	c.mtx.Lock()
	c.flushBucketHandles()
	c.mtx.Unlock()
	return err
}

// flushBucketHandles cancels all bucket handles and waits for them to complete.
// returns when all handles have finished execution
// bucketMtx should be locked by the caller.
func (c *Controller) flushBucketHandles() {
	for k, v := range c.bucketHandles {
		v.Flush()
		delete(c.bucketHandles, k)
	}
}

// flushBucketHandle flushes a bucket handle for a particular bucket id
// returns when all handles have finished execution
// bucketMtx should be locked by the caller.
func (c *Controller) flushBucketHandle(bucketID string) {
	v, ok := c.bucketHandles[bucketID]
	if !ok {
		return
	}
	v.Flush()
	delete(c.bucketHandles, bucketID)
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case volume.LookupVolume:
		return directive.R(c.resolveLookupVolume(ctx, di, d))
	case bucket.ApplyBucketConfig:
		return directive.R(c.resolveApplyBucketConf(ctx, di, d))
	case volume.ListBuckets:
		return directive.R(c.resolveListBuckets(ctx, di, d))
	case volume.BuildBucketAPI:
		return directive.R(c.resolveBuildBucketAPI(ctx, di, d))
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
