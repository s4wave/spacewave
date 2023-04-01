package bldr_plugin_forward_rpc_service

import (
	"context"

	bifrost_rpc_access "github.com/aperturerobotics/bifrost/rpc/access"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/cenkalti/backoff"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/plugin/forward-rpc-service"

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
	var bo backoff.BackOff
	if boc := conf.GetBackoff(); !boc.GetEmpty() {
		bo = boc.Construct()
	}
	c.ClientController = bifrost_rpc_access.NewClientController(
		le,
		controller.NewInfo(
			ControllerID,
			Version,
			"forwards rpc calls to plugin: "+c.conf.GetPluginId(),
		),
		c.PluginLoadAccessClient,
		serviceIdRe,
		serverIdRe,
		false,
		bo,
	)
	return c
}

// PluginLoadAccessClient adds a reference to the plugin and waits for it to be built.
func (c *Controller) PluginLoadAccessClient(
	ctx context.Context,
	cb func(
		ctx context.Context,
		client bifrost_rpc_access.SRPCAccessRpcServiceClient,
	) error,
) error {
	return bldr_plugin.ExPluginLoadAccessClient(
		ctx,
		c.bus,
		c.conf.GetPluginId(),
		func(ctx context.Context, client srpc.Client) error {
			return cb(
				ctx,
				bifrost_rpc_access.NewSRPCAccessRpcServiceClient(client),
			)
		},
	)
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
