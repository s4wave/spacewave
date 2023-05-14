package block_store_http_lookup

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	block_store_http "github.com/aperturerobotics/hydra/block/store/http"
	"github.com/aperturerobotics/hydra/dex"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/blang/semver"
)

// ControllerID is the controller id.
const ControllerID = "hydra/block/store/http/lookup"

// Version is the API version.
var Version = semver.MustParse("0.0.1")

// Controller looks up blocks via an HTTP service for LookupBlockFromNetwork directives.
type Controller struct {
	// conf is the config
	conf *Config
	// store is the http-backed block store
	store *ccontainer.CContainer[*block_store_http.HTTPBlock]
}

// NewController constructs a controller that looks up blocks via an HTTP
// service for LookupBlockFromNetwork directives.
func NewController(conf *Config) *Controller {
	return &Controller{
		conf:  conf,
		store: ccontainer.NewCContainer[*block_store_http.HTTPBlock](nil),
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"lookup blocks via http",
	)
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	baseURL, err := c.conf.ParseURL()
	if err != nil {
		return err
	}
	store := block_store_http.NewHTTPBlock(false, http.DefaultClient, baseURL, 0)
	c.store.SetValue(store)
	return nil
}

// GetHTTPStore returns the http store.
func (c *Controller) GetHTTPStore(ctx context.Context) (*block_store_http.HTTPBlock, error) {
	return c.store.WaitValue(ctx, nil)
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns resolver(s). If not, returns nil.
// It is safe to add a reference to the directive during this call.
// The context passed is canceled when the directive instance expires.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case dex.LookupBlockFromNetwork:
		return c.resolveLookupBlockFromNetwork(ctx, di, d)
	}
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	c.store.SetValue(nil)
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
