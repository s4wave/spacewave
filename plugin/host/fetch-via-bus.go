package plugin_host

import (
	"context"

	"github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/sirupsen/logrus"
)

// PluginFetchViaBus implements the PluginFetch service.
type PluginFetchViaBus struct {
	le *logrus.Entry
	b  bus.Bus
}

// NewPluginFetchViaBus constructs a new PluginFetchViaBus implementation.
func NewPluginFetchViaBus(le *logrus.Entry, b bus.Bus) *PluginFetchViaBus {
	return &PluginFetchViaBus{le: le, b: b}
}

// FetchPlugin fetches a plugin by id.
func (f *PluginFetchViaBus) FetchPlugin(
	ctx context.Context,
	req *plugin.FetchPluginRequest,
) (*plugin.FetchPluginResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	pluginID := req.GetPluginId()
	f.le.Infof("plugin host requests fetching plugin: %s", pluginID)

	return ExFetchPlugin(ctx, f.b, pluginID, false)
}

// _ is a type assertion
var _ plugin.SRPCPluginFetchServer = ((*PluginFetchViaBus)(nil))
