package plugin_host_controller

import (
	"context"
	"slices"

	bldr_plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/sirupsen/logrus"
)

// Controller implements the PluginHost controller.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// info contains the controller info
	info *controller.Info
	// host is the plugin host
	host bldr_plugin_host.PluginHost
	// platformID is the host platform id
	platformID string
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	info *controller.Info,
	host bldr_plugin_host.PluginHost,
) *Controller {
	return &Controller{
		le:         le,
		bus:        bus,
		info:       info,
		host:       host,
		platformID: host.GetPlatformId(),
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// GetPluginHost returns the plugin host.
func (c *Controller) GetPluginHost() bldr_plugin_host.PluginHost {
	return c.host
}

// Execute executes the controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) (rerr error) {
	c.le.Info("starting plugin host")
	return c.host.Execute(ctx)
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
// The context tasked is canceled when the directive instance expires.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case bldr_plugin_host.LookupPluginHost:
		return directive.R(c.resolveLookupPluginHost(d))
	}
	return nil, nil
}

// resolveLookupPluginHost returns a resolver for looking up the plugin host.
func (c *Controller) resolveLookupPluginHost(
	dir bldr_plugin_host.LookupPluginHost,
) (directive.Resolver, error) {
	// check if we can immediately reject this directive
	matchPlatformIDs := dir.LookupPluginHostPlatformIDs()
	if len(matchPlatformIDs) != 0 {
		if !slices.Contains(matchPlatformIDs, c.platformID) {
			return nil, nil
		}
	}

	// resolve with the host
	return directive.NewValueResolver([]bldr_plugin_host.LookupPluginHostValue{c.host}), nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
