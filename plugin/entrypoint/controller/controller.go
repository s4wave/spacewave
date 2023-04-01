package plugin_entrypoint_controller

import (
	"context"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/plugin/entrypoint/controller"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller implements the plugin entrypoint controller.
// Handles directives from the plugin bus forwarding RPCs to the host.
// Ex: LoadPlugin, LookupRpcClient<plugin/foo/ or plugin-host/>
type Controller struct {
	// b is the bus
	b bus.Bus
	// le is the logger
	le *logrus.Entry
	// srv is the service client
	srv bldr_plugin.SRPCPluginHostClient
	// hostPrefixClient is a srpc client for the plugin host
	// matches & strips the plugin-host/ prefix from service ids.
	hostPrefixClient srpc.Client
}

// NewController constructs the plugin entrypoint controller.
func NewController(b bus.Bus, le *logrus.Entry, srv bldr_plugin.SRPCPluginHostClient) *Controller {
	return &Controller{
		b:                b,
		le:               le,
		srv:              srv,
		hostPrefixClient: srpc.NewPrefixClient(srv.SRPCClient(), []string{bldr_plugin.HostServiceIDPrefix}),
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "handles plugin directives")
}

// GetPluginHostClient returns the plugin host client.
func (c *Controller) GetPluginHostClient() bldr_plugin.SRPCPluginHostClient {
	return c.srv
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns resolver(s). If not, returns nil.
// It is safe to add a reference to the directive during this call.
// The context passed is canceled when the directive instance expires.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case bldr_plugin.LoadPlugin:
		return directive.R(c.resolveLoadPlugin(ctx, di, dir))
	case bifrost_rpc.LookupRpcClient:
		return directive.R(c.resolveLookupRpcClient(ctx, di, dir))
	}
	return nil, nil
}

// BuildRemotePluginClient builds a client for the remote plugin.
func (c *Controller) BuildRemotePluginClient(pluginID string, waitAck bool) srpc.Client {
	return rpcstream.NewRpcStreamClient(c.srv.PluginRpc, pluginID, waitAck)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
