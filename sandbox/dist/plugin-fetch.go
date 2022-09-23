package main

import (
	"context"

	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/sirupsen/logrus"
)

// PluginFetch implements the PluginFetch service.
type PluginFetch struct {
	le *logrus.Entry
	b  bus.Bus
}

// NewPluginFetch constructs a new PluginFetch implementation.
func NewPluginFetch(le *logrus.Entry, b bus.Bus) *PluginFetch {
	return &PluginFetch{le: le, b: b}
}

// FetchPlugin fetches a plugin by id.
func (f *PluginFetch) FetchPlugin(
	ctx context.Context,
	req *plugin.FetchPluginRequest,
) (*plugin.FetchPluginResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	pluginID := req.GetPluginId()
	f.le.Infof("plugin host requests fetching plugin: %s", pluginID)

	return plugin_host.ExFetchPlugin(ctx, f.b, pluginID, false)
}

// _ is a type assertion
var _ plugin.SRPCPluginFetchServer = ((*PluginFetch)(nil))
