package node_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	block_store "github.com/aperturerobotics/hydra/block/store"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/node"
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
	// blockStores is the list of available block stores.
	// keyed by store ID
	blockStores map[string]block_store.Store
	// buckets tracks the list of loadedBucket trackers.
	// the bucket trackers manage cross-bucket-store lookups.
	// key: bucket id
	buckets *keyed.KeyedRefCount[string, *loadedBucket]
}

// NewController constructs a new node controller.
func NewController(cc *Config, le *logrus.Entry, b bus.Bus) *Controller {
	ctrl := &Controller{
		le: le,
		b:  b,
		cc: cc,

		blockStores: make(map[string]block_store.Store),
	}
	ctrl.buckets = keyed.NewKeyedRefCount(ctrl.newLoadedBucket)
	return ctrl
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// execute block store monitoring.
	_, vRef, err := c.b.AddDirective(
		block_store.NewLookupBlockStore(""),
		newBlockStoreRefHandler(c),
	)
	if err != nil {
		return err
	}

	c.buckets.SetContext(ctx, true)
	_ = context.AfterFunc(ctx, vRef.Release)
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
