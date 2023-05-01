package node_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/node"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "hydra/node"

// Controller is the Node controller.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// b is the controller bus
	b bus.Bus
	// cc is the configuration
	cc *Config
	// mtx guards the maps
	mtx sync.Mutex
	// volumes is the list of available volume handles.
	// keyed by volume ID
	volumes map[string]volume.Volume
	// buckets tracks the list of loadedBucket trackers.
	// the bucket trackers manage cross-volume lookups.
	// key: bucket id
	buckets *keyed.KeyedRefCount[string, *loadedBucket]
}

// NewController constructs a new node controller.
func NewController(cc *Config, le *logrus.Entry, b bus.Bus) *Controller {
	ctrl := &Controller{
		le: le,
		b:  b,
		cc: cc,

		volumes: make(map[string]volume.Volume),
	}
	ctrl.buckets = keyed.NewKeyedRefCount(ctrl.newLoadedBucket)
	return ctrl
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

	c.buckets.SetContext(ctx, true)
	<-ctx.Done()

	vRef.Release()
	c.buckets.SetContext(nil, false)

	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	if !c.cc.GetDisableLookup() {
		dir := di.GetDirective()
		switch d := dir.(type) {
		case bucket_lookup.BuildBucketLookup:
			return directive.R(c.resolveBuildBucketLookup(ctx, di, d))
		}
	}

	return nil, nil
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
