// Package plugin_host_static resolves LookupPluginHost with a fixed set of
// plugin hosts that another component runs.
package plugin_host_static

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
)

// ControllerID is the static plugin host controller ID.
const ControllerID = "bldr/plugin/host/static"

// Version is the static plugin host controller version.
var Version = controller.MustParseVersion("0.0.1")

// Controller resolves LookupPluginHost with a fixed set of plugin hosts. It
// publishes the hosts without running them; their owner runs them.
type Controller struct {
	// hosts is the published host set.
	hosts []plugin_host.PluginHost
}

// NewController constructs a controller that publishes hosts.
func NewController(hosts []plugin_host.PluginHost) *Controller {
	return &Controller{hosts: slices.Clone(hosts)}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "static plugin hosts")
}

// Execute executes the controller.
func (c *Controller) Execute(context.Context) error {
	return nil
}

// HandleDirective resolves LookupPluginHost with the hosts matching its
// platform IDs.
func (c *Controller) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(plugin_host.LookupPluginHost)
	if !ok {
		return nil, nil
	}

	platformIDs := dir.LookupPluginHostPlatformIDs()
	var hosts []plugin_host.PluginHost
	for _, host := range c.hosts {
		if len(platformIDs) == 0 || slices.Contains(platformIDs, host.GetPlatformId()) {
			hosts = append(hosts, host)
		}
	}
	if len(hosts) == 0 {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver(hosts), nil)
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
