package manifest_fetch_rpc

import (
	"context"
	"regexp"

	manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/manifest/fetch/rpc"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller fetches Manifests via the ManifestFetch service.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// fetchManifestIdRe is the parsed regex to filter manifest by.
	// if nil, accepts any
	fetchManifestIdRe *regexp.Regexp
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	// note: checked in Validate()
	manifestIdRe, _ := conf.ParseFetchManifestIdRe()
	return &Controller{
		le:                le,
		bus:               bus,
		conf:              conf,
		fetchManifestIdRe: manifestIdRe,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"fetches manifests via rpc",
	)
}

// Execute executes the controller.
// Returning nil ends execution.
func (c *Controller) Execute(rctx context.Context) (rerr error) {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case manifest.FetchManifest:
		return directive.R(c.resolveFetchManifest(ctx, inst, d))
	}
	return nil, nil
}

// FetchManifest fetches a manifest, yielding the FetchManifestResponse.
// Loads the configured plugin and uses its RPC service to fetch.
// if returnIfIdle is set, returns an error if the directive becomes idle (not found)
// Returns if context is canceled.
func (c *Controller) FetchManifest(
	ctx context.Context,
	dir manifest.FetchManifest,
	hnd directive.ResolverHandler,
	returnOnIdle bool,
) error {
	return manifest.FetchManifestViaRpcLookupClientSet(
		ctx,
		c.bus,
		dir,
		c.conf.GetServiceId(),
		c.conf.GetClientId(),
		true,
		hnd,
		returnOnIdle,
		c.le,
	)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
