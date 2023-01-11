package plugin_fetch_viaplugin

import (
	"context"
	"regexp"

	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/plugin/host/fetch/via-plugin"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller fetches plugins via the PluginFetch service on a loaded plugin.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// fetchPluginIdRe is the parsed regex to filter requests by.
	// if nil, accepts any
	fetchPluginIdRe *regexp.Regexp
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	// note: checked in Validate()
	pluginIdRe, _ := conf.ParseFetchPluginIdRegex()
	return &Controller{
		le:              le,
		bus:             bus,
		conf:            conf,
		fetchPluginIdRe: pluginIdRe,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"fetches plugins via plugin: "+c.conf.GetPluginId(),
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
	case plugin_host.FetchPlugin:
		return directive.R(c.resolveFetchPlugin(ctx, inst, d))
	}
	return nil, nil
}

// FetchPlugin fetches a plugin, yielding the FetchPluginResponse.
// Loads the configured plugin and uses its RPC service to fetch.
// if returnIfIdle is set, returns an error if the directive becomes idle (not found)
// Returns if context is canceled.
func (c *Controller) FetchPlugin(
	ctx context.Context,
	pluginID string,
	returnIfIdle bool,
) (*plugin.FetchPluginResponse, error) {
	var resp *plugin.FetchPluginResponse
	err := plugin_host.ExPluginLoadAccessClient(
		ctx,
		c.bus,
		returnIfIdle,
		c.conf.GetPluginId(),
		func(ctx context.Context, client srpc.Client) error {
			c.le.Debugf("fetching plugin %s via plugin %s", pluginID, c.conf.GetPluginId())
			fetchClient := plugin.NewSRPCPluginFetchClient(client)
			var err error
			resp, err = fetchClient.FetchPlugin(ctx, &plugin.FetchPluginRequest{
				PluginId: pluginID,
			})
			return err
		},
	)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
