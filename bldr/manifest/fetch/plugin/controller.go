package manifest_fetch_plugin

import (
	"context"
	"regexp"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/manifest/fetch/plugin"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller fetches Manifests via the ManifestFetch service on a loaded plugin.
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
		"fetches manifests via plugin: "+c.conf.GetPluginId(),
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

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
