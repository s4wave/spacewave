package plugin_host_scheduler

import (
	"context"
	"slices"

	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
)

// GetHostState returns host identities, a wait for this snapshot to change,
// and the host lookup failure. The scheduler owns the lookup and its lifetime.
func (c *Controller) GetHostState() ([]plugin_host.PluginHost, func(context.Context) error, error) {
	state := c.pluginHostsCtr.GetValue()
	wait := func(ctx context.Context) error {
		_, err := c.pluginHostsCtr.WaitValueChange(ctx, state, nil)
		return err
	}
	if state == nil {
		return nil, wait, nil
	}
	return slices.Clone(state.pluginHosts), wait, state.err
}
