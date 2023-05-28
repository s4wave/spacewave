package block_store_rpc_lookup

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	block_store_rpc "github.com/aperturerobotics/hydra/block/store/rpc"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller id.
const ControllerID = "hydra/block/store/rpc/lookup"

// Version is the API version.
var Version = semver.MustParse("0.0.1")

// Controller looks up blocks via an RPC service for LookupBlockFromNetwork directives.
type Controller struct {
	// conf is the config
	conf *Config
	// blockStoreCtrl is an internal block store controller
	blockStoreCtrl *block_store_rpc.Controller
}

// NewController constructs a controller that looks up blocks via an HTTP
// service for LookupBlockFromNetwork directives.
func NewController(b bus.Bus, le *logrus.Entry, conf *Config) *Controller {
	return &Controller{
		conf: conf,
		blockStoreCtrl: block_store_rpc.NewController(b, le, &block_store_rpc.Config{
			ServiceId:    conf.GetServiceId(),
			ClientId:     conf.GetClientId(),
			ReadOnly:     true,
			BucketIds:    []string{conf.GetBucketId()},
			SkipNotFound: conf.GetSkipNotFound(),
			Verbose:      conf.GetVerbose(),
		}),
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"lookup blocks via rpc",
	)
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	return c.blockStoreCtrl.Execute(ctx)
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns resolver(s). If not, returns nil.
// It is safe to add a reference to the directive during this call.
// The context passed is canceled when the directive instance expires.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return c.blockStoreCtrl.HandleDirective(ctx, di)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return c.blockStoreCtrl.Close()
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
