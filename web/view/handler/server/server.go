package web_view_handler_server

import (
	"context"
	"strings"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	web_view "github.com/aperturerobotics/bldr/web/view"
	web_view_handler "github.com/aperturerobotics/bldr/web/view/handler"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
)

// ControllerID is the controller id.
const ControllerID = "bldr/web/view/handler/server"

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "handle web view rpc server"

// Controller is the handle web view server controller.
// Handles incoming HandleWebView requests.
// Determines sender of HandleWebView from the LookupRpcService server id.
// Forwards WebView RPC calls to AccessWebView service on the sender.
type Controller struct {
	*bus.BusController[*Config]
}

// NewFactory constructs the controller factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{BusController: base}, nil
		},
	)
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		return c.resolveLookupRpcService(ctx, di, dir)
	}

	return nil, nil
}

// lookupRpcServiceResolver resolves the LookupRpcService directive
type lookupRpcServiceResolver struct {
	c                       *Controller
	accessWebViewsServiceID string
}

// resolveLookupRpcService returns a resolver for the LookupRpcService directive.
func (c *Controller) resolveLookupRpcService(
	_ context.Context,
	_ directive.Instance,
	dir bifrost_rpc.LookupRpcService,
) ([]directive.Resolver, error) {
	serviceID, serverID := dir.LookupRpcServiceID(), dir.LookupRpcServerID()
	if serviceID != web_view_handler.SRPCHandleWebViewServiceServiceID {
		return nil, nil
	}

	// this logic determines who to call for AccessWebView service.
	// the server ID is in two formats:
	//  - plugin-host/{server-id}
	//  - plugin/{plugin-id}/{server-id}
	// depending on the format we can add a prefix to the service id
	//  - plugin/{plugin-id}/{service-id}
	//  - plugin-host/{service-id}
	var targetServiceID string
	if serverID == bldr_plugin.HostServerIDPrefix[:len(bldr_plugin.HostServerIDPrefix)-1] ||
		strings.HasPrefix(serverID, bldr_plugin.HostServerIDPrefix) {
		targetServiceID = bldr_plugin.HostServiceIDPrefix + web_view.SRPCAccessWebViewsServiceID
	} else if strings.HasPrefix(serverID, bldr_plugin.PluginServerIDPrefix) {
		// parse the plugin id from the server id
		pluginID, _, _ := strings.Cut(serverID[len(bldr_plugin.PluginServerIDPrefix):], "/")
		if err := bldr_plugin.ValidatePluginID(pluginID, false); err != nil {
			// ignore it: invalid plugin id
			return nil, errors.Wrap(err, "server id contains invalid plugin id")
		}
		// call via the plugin
		targetServiceID = bldr_plugin.PluginServiceIDPrefix + pluginID + "/" + web_view.SRPCAccessWebViewsServiceID
	} else {
		return nil, nil
	}

	return directive.R(&lookupRpcServiceResolver{
		c:                       c,
		accessWebViewsServiceID: targetServiceID,
	}, nil)
}

// Resolve resolves the values, emitting them to the handler.
func (r *lookupRpcServiceResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	handler.ClearValues()

	client, _, clientRef, err := bifrost_rpc.ExLookupRpcClientSet(ctx, r.c.GetBus(), r.accessWebViewsServiceID, ControllerID, true, nil)
	if err != nil {
		return err
	}
	defer clientRef.Release()

	accessClient := web_view.NewSRPCAccessWebViewsClientWithServiceID(
		client,
		r.accessWebViewsServiceID,
	)
	handleViaBus := web_view_handler.NewHandleWebViewViaBus(r.c.GetLogger(), r.c.GetBus(), accessClient)

	mux := srpc.NewMux()
	_ = web_view_handler.SRPCRegisterHandleWebViewService(mux, handleViaBus)
	var value bifrost_rpc.LookupRpcServiceValue = mux
	_, _ = handler.AddValue(value)
	handler.MarkIdle(true)

	<-ctx.Done()
	return context.Canceled
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
