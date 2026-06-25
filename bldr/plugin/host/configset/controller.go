package plugin_host_configset

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
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
func (c *Controller) Execute(ctx context.Context) (rerr error) {
	le := c.GetLogger()
	configSet := c.GetConfig().GetConfigSet()
	if len(configSet) == 0 {
		return nil
	}

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

	le.Debugf("applying configset with %d configs to plugin host", len(configSet))
	hostClient := hostClients[0]
	pluginHostClient := plugin.NewSRPCPluginHostClientWithServiceID(hostClient, serviceID)
	status, err := pluginHostClient.ExecController(ctx, &controller_exec.ExecControllerRequest{
		ConfigSet: &configset_proto.ConfigSet{Configs: configSet},
	})
	if err != nil {
		if isWebRuntimeClientClosed(err) {
			return nil
		}
		return err
	}
	for {
		resp, err := status.Recv()
		if err != nil {
			if isWebRuntimeClientClosed(err) {
				return nil
			}
			return err
		}
		if logStr := resp.FormatLogString(); logStr != "" {
			le.Debug(logStr)
		}
	}
}

func isWebRuntimeClientClosed(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "WebRuntimeClientInstance closed:") {
		return true
	}
	const clientPrefix = "WebRuntimeClient: "
	clientIdx := strings.Index(msg, clientPrefix)
	if clientIdx < 0 {
		return false
	}
	msg = msg[clientIdx+len(clientPrefix):]

	const generationMarker = ": runtime client generation "
	_, after, ok := strings.Cut(msg, generationMarker)
	if !ok {
		return false
	}
	generation := after

	const normalCloseSuffix = " closed: normal-close"
	if !strings.HasSuffix(generation, normalCloseSuffix) {
		return false
	}
	generation = strings.TrimSuffix(generation, normalCloseSuffix)
	if generation == "" {
		return false
	}
	for _, r := range generation {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
