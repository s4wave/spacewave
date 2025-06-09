package plugin_entrypoint_controller

import (
	"context"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
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
	// meta is the plugin meta
	meta *bldr_plugin.PluginMeta
	// srv is the service client
	srv bldr_plugin.SRPCPluginHostClient
	// hostClient is a client for the plugin host
	hostClient srpc.Client
	// loopbackClient is a client for the local plugin
	loopbackClient srpc.Client
}

// NewController constructs the plugin entrypoint controller.
func NewController(b bus.Bus, le *logrus.Entry, meta *bldr_plugin.PluginMeta, srv bldr_plugin.SRPCPluginHostClient) *Controller {
	loopbackClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(bifrost_rpc.NewInvoker(b, "plugin/"+meta.GetPluginId(), true))))
	return &Controller{
		b:              b,
		le:             le,
		meta:           meta,
		srv:            srv,
		hostClient:     srv.SRPCClient(),
		loopbackClient: loopbackClient,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "handles plugin directives")
}

// GetPluginMeta returns a copy of the plugin meta object.
func (c *Controller) GetPluginMeta() *bldr_plugin.PluginMeta {
	return c.meta.CloneVT()
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
		return directive.R(bldr_plugin.ResolveLookupRpcClient(ctx, dir, c))
	case unixfs_access.AccessUnixFS:
		return directive.R(bldr_plugin.ResolveAccessUnixfs(ctx, dir, c))
	}
	return nil, nil
}

// BuildRemotePluginClient builds a client for the remote plugin.
func (c *Controller) BuildRemotePluginClient(pluginID string, waitAck bool) srpc.Client {
	return rpcstream.NewRpcStreamClient(c.srv.PluginRpc, pluginID, waitAck)
}

// WaitPluginHostClient waits for an RPC client for the plugin host.
//
// Released is a function to call if the client becomes invalid.
// Returns nil, nil, err if any error.
// Returns nil, nil, nil to skip resolving the client.
// Otherwise returns client, releaseFunc, nil
func (c *Controller) WaitPluginHostClient(ctx context.Context, released func()) (srpc.Client, func(), error) {
	return c.hostClient, nil, nil
}

// WaitPluginClient waits for an RPC client for a plugin.
//
// if pluginID is invalid, returns an error.
//
// Released is a function to call if the client becomes invalid.
// Returns nil, nil, err if any error.
// Returns nil, nil, nil to skip resolving the client.
// Otherwise returns client, releaseFunc, nil
func (c *Controller) WaitPluginClient(ctx context.Context, released func(), pluginID string) (srpc.Client, func(), error) {
	if pluginID == c.meta.GetPluginId() {
		return c.loopbackClient, nil, nil
	}
	if err := bldr_plugin.ValidatePluginID(pluginID, false); err != nil {
		return nil, nil, err
	}
	client, ref, err := bldr_plugin.ExPluginLoadWaitClient(ctx, c.b, pluginID, released)
	if err != nil {
		return nil, nil, err
	}
	return client, ref.Release, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller              = ((*Controller)(nil))
	_ bldr_plugin.LookupRpcClientHandler = ((*Controller)(nil))
)
