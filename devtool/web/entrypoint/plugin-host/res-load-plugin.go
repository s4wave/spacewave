package devtool_web_entrypoint_plugin_host

import (
	"context"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/directive"
)

// resolveLoadPlugin resolves a LoadPlugin directive.
func (c *Controller) resolveLoadPlugin(
	ctx context.Context,
	di directive.Instance,
	dir bldr_plugin.LoadPlugin,
) (directive.Resolver, error) {
	return plugin_host.NewLoadPluginResolver(c, dir.LoadPluginID()), nil
}
