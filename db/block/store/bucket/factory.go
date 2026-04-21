package block_store_bucket

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/blang/semver/v4"
)

// Factory constructs the controller.
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
}

// NewFactory builds a factory.
func NewFactory(bus bus.Bus) *Factory {
	return &Factory{bus: bus}
}

// GetConfigID returns the configuration ID for the controller.
func (t *Factory) GetConfigID() string {
	return ConfigID
}

// GetControllerID returns the unique ID for the controller.
func (t *Factory) GetControllerID() string {
	return ControllerID
}

// ConstructConfig constructs an instance of the controller configuration.
func (t *Factory) ConstructConfig() config.Config {
	return &Config{}
}

// Construct constructs the associated controller given configuration.
func (t *Factory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	// le := opts.GetLogger()
	cc := conf.(*Config)

	bucketStoreID := cc.GetBucketStoreId()
	if bucketStoreID == "" {
		// default to block store id
		bucketStoreID = cc.GetBlockStoreId()
	}

	accessBlockStore := block_store.NewAccessBlockStoreViaBusFunc(
		t.bus,
		cc.GetBlockStoreId(),
		cc.GetNotFoundIfIdle(),
	)

	return NewController(bucketStoreID, cc.GetBucketConfig(), accessBlockStore), nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() semver.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = ((*Factory)(nil))
