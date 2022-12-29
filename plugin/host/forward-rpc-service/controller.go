package plugin_host_forward_rpc_service

import (
	"context"

	bifrost_rpc_access "github.com/aperturerobotics/bifrost/rpc/access"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/plugin/host/forward-rpc-service"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller forwards RPC services to a remote plugin.
type Controller struct {
	*bifrost_rpc_access.ClientController
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	// note: checked in Validate()
	serviceIdRe, _ := conf.ParseServiceIdRegex()
	serverIdRe, _ := conf.ParseServerIdRegex()
	c := &Controller{
		bus:  bus,
		conf: conf,
	}
	c.ClientController = bifrost_rpc_access.NewClientController(
		controller.NewInfo(
			ControllerID,
			Version,
			"forwards rpc calls to plugin: "+c.conf.GetPluginId(),
		),
		c.PluginLoadWaitClient,
		serviceIdRe,
		serverIdRe,
	)
	return c
}

// PluginLoadWaitClient adds a reference to the plugin and waits for client to be built.
func (c *Controller) PluginLoadWaitClient(ctx context.Context) (*bifrost_rpc_access.SRPCAccessRpcServiceClient, func(), error) {
	// load / attach to the plugin
	rpcClient, rpcClientRef, err := plugin_host.ExPluginLoadWaitClient(
		ctx,
		c.bus,
		c.conf.GetPluginId(),
	)
	if err != nil {
		return nil, nil, err
	}
	accessClient := bifrost_rpc_access.NewSRPCAccessRpcServiceClient(rpcClient)
	return &accessClient, rpcClientRef.Release, nil
}

// _ is a type assertion
var (
	_ controller.Controller              = ((*Controller)(nil))
	_ bifrost_rpc_access.BuildClientFunc = ((*Controller)(nil).PluginLoadWaitClient)
)
