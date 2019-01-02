package volume_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/store"
	volume "github.com/aperturerobotics/hydra/volume"
	"github.com/sirupsen/logrus"
)

// Controller implements a common volume controller.
// The controller looks up the peer, acquires its identity, constructs the
// transport, and manages the lifecycle of dialing and accepting links.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// ctor is the constructor
	ctor volume.Constructor
	// volumeCh contains the controlled volume
	volumeCh chan volumeCtxPair
	// controllerInfo contains the controller info
	controllerInfo controller.Info

	// reconcilersMtx locks the reconcilers map
	reconcilersMtx sync.Mutex
	// reconcilers contains running reconciler instances.
	reconcilers map[bucket_store.BucketReconcilerPair]*runningReconciler

	// bucketMtx locks bucket requests and processing
	bucketMtx sync.Mutex
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
	bus bus.Bus,
	info controller.Info,
	ctor volume.Constructor,
) *Controller {
	return &Controller{
		le:             le,
		bus:            bus,
		ctor:           ctor,
		volumeCh:       make(chan volumeCtxPair, 1),
		reconcilers:    make(map[bucket_store.BucketReconcilerPair]*runningReconciler),
		bucketHandles:  make(map[string]*bucketHandle),
		controllerInfo: info,
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

	c.le = c.le.WithField("peer-id", v.GetPeerID().Pretty())
	c.le.Debug("volume constructed, initializing")

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
	if err := c.wakeFilledReconcilerQueues(ctx, v); err != nil {
		c.le.WithError(err).Warn("unable to list filled bucket reconciler queues")
	}

	c.volumeCh <- volumeCtxPair{
		ctx: volCtx,
		vol: v,
	}
	c.le.Info("volume ready")

	select {
	case <-ctx.Done():
		err = ctx.Err()
	case err = <-errCh:
	}

	c.bucketMtx.Lock()
	c.flushBucketHandles()
	c.bucketMtx.Unlock()
	return err
}

// flushBucketHandles cancels all bucket handles and waits for them to complete.
// returns when all handles have finished execution
// bucketMtx should be locked by the caller.
func (c *Controller) flushBucketHandles() {
	for _, v := range c.bucketHandles {
		v.ctxCancel()
	}
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
) (directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case peer.GetPeer:
		return newGetPeerResolver(c, d), nil
	case volume.LookupVolume:
		return c.resolveLookupVolume(ctx, di, d)
	case bucket.ApplyBucketConfig:
		return c.resolveApplyBucketConf(ctx, di, d)
	case volume.ListBuckets:
		return c.resolveListBuckets(ctx, di, d)
	case volume.BuildBucketAPI:
		return c.resolveBuildBucketAPI(ctx, di, d)
	}

	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() controller.Info {
	return c.controllerInfo
}

// GetVolume returns the controlled volume.
// This may wait for the volume to be ready.
func (c *Controller) GetVolume(ctx context.Context) (volume.Volume, error) {
	var r volume.Volume
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case rv := <-c.volumeCh:
		r = rv.vol
		c.volumeCh <- rv
	}

	return r, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ volume.Controller = ((*Controller)(nil))
