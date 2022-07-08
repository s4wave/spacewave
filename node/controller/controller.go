package node_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/node"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "hydra/node/1"

// Controller is the Node controller.
// It implements node.Node as a controller.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// b is the controller bus
	b bus.Bus
	// cc is the configuration
	cc *Config
	// bucketWake wakes the running bucket watcher
	bucketWake chan struct{}
	// mtx guards the maps
	mtx sync.Mutex
	// volumes is the list of available volume handles.
	// keyed by volume ID
	volumes map[string]volume.Volume
	// buckets are loaded buckets
	buckets map[string]*loadedBucket
}

// NewController constructs a new node controller.
func NewController(cc *Config, le *logrus.Entry, b bus.Bus) (*Controller, error) {
	return &Controller{
		le: le,
		b:  b,
		cc: cc,

		bucketWake: make(chan struct{}, 1),
		volumes:    make(map[string]volume.Volume),
		buckets:    make(map[string]*loadedBucket),
	}, nil
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// execute volume monitoring.
	_, vRef, err := c.b.AddDirective(
		volume.NewLookupVolume("", peer.ID("")),
		newVolumeRefHandler(c),
	)
	if err != nil {
		return err
	}
	defer vRef.Release()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.bucketWake:
		}

		// Manage bucket execution contexts.
		c.mtx.Lock()
		for bk := range c.buckets {
			b := c.buckets[bk]
			b.mtx.Lock()
			if len(b.refs) == 0 {
				delete(c.buckets, bk)
				b.ctxCancel()
				b.mtx.Unlock()
				continue
			}
			b.mtx.Unlock()

			if b.ctxCancel == nil {
				nctx, nctxCancel := context.WithCancel(ctx)
				b.ctxCancel = nctxCancel
				go func() {
					if err := b.Execute(nctx); err != nil {
						if err != context.Canceled {
							c.le.WithError(err).Warn("bucket exited with error")
						}
					}
					c.mtx.Lock()
					if v, ok := c.buckets[b.bucketID]; ok && v == b {
						delete(c.buckets, b.bucketID)
					}
					c.mtx.Unlock()
				}()
			}
		}
		c.mtx.Unlock()
	}
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) (directive.Resolver, error) {
	if !c.cc.GetDisableLookup() {
		dir := di.GetDirective()
		switch d := dir.(type) {
		case bucket_lookup.BuildBucketLookup:
			return c.resolveBuildBucketLookup(ctx, di, d)
		case bucket.ApplyBucketConfig:
			c.handleApplyBucketConfig(ctx, di, d)
			return nil, nil
		}
	}

	return nil, nil
}

// procBucketWake wakes the bucketWake ch
func (c *Controller) procBucketWake() {
	select {
	case c.bucketWake <- struct{}{}:
	default:
	}
}

// flushBucketVolume flushes volume handles for a particular bucket/volume
// combination and forces a re-check of the volume bucket config.
func (c *Controller) flushBucketVolume(bucketID, volumeID string) {
	c.mtx.Lock()
	if b, ok := c.buckets[bucketID]; ok {
		b.ClearVolume(volumeID)
		b.PushVolume(volumeID)
	}
	c.mtx.Unlock()
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"node controller",
	)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ node.Controller = ((*Controller)(nil))
