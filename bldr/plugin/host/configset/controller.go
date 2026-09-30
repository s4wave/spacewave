package plugin_host_configset

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/plugin/host/configset"

// Version is the version of this controller.
var Version = controller.MustParseVersion("0.0.1")

// Controller applies a config set to the plugin host.
type Controller struct {
	*bus.BusController[*Config]
}

// NewFactory constructs the factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		"applies configset to the plugin host",
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{BusController: base}, nil
		},
	)
}

// Execute executes the controller.
// Returning nil ends execution.
func (c *Controller) Execute(ctx context.Context) error {
	// Read the logger and the config set to apply.
	le := c.GetLogger()
	configSet := c.GetConfig().GetConfigSet()

	// An empty config set has nothing to apply.
	if len(configSet) == 0 {
		return nil
	}

	// Look up the plugin host RPC client.
	serviceID := plugin.HostServiceIDPrefix + plugin.SRPCPluginHostServiceID
	hostClients, _, hostClientRef, err := bifrost_rpc.ExLookupRpcClient(
		ctx,
		c.GetBus(),
		serviceID,
		ControllerID,
		true,
		nil,
	)
	if err != nil {
		return err
	}
	defer hostClientRef.Release()

	// Log the config set size before applying it.
	le.Debugf("applying configset with %d configs to plugin host", len(configSet))

	// Execute the config set through the plugin host.
	hostClient := hostClients[0]
	pluginHostClient := plugin.NewSRPCPluginHostClientWithServiceID(hostClient, serviceID)
	status, err := pluginHostClient.ExecController(ctx, &controller_exec.ExecControllerRequest{
		ConfigSet: &configset_proto.ConfigSet{Configs: configSet},
	})
	if err != nil {
		if web_runtime.IsWebRuntimeClientClosed(err) {
			return nil
		}
		return err
	}
	defer status.Close()

	// Stream the exec status until it closes or reports an error.
	for {
		resp, err := status.Recv()
		if err != nil {
			if web_runtime.IsWebRuntimeClientClosed(err) {
				return nil
			}
			return err
		}
		// Log the status and stop on a reported error.
		if logStr := resp.FormatLogString(); logStr != "" {
			le.Debug(logStr)
		}
		if err := resp.GetError(); err != nil {
			return err
		}
	}
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
