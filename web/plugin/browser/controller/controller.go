package bldr_web_plugin_browser_controller

import (
	"context"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	bldr_web_plugin_browser "github.com/aperturerobotics/bldr/web/plugin/browser"
	web_view "github.com/aperturerobotics/bldr/web/view"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/web/plugin/browser/controller"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller manages running the web plugin browser host.
// Serves the WebPluginBrowserHost RPC service.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// mux is the rpc mux for the WebPluginBrowserHost RPC service.
	mux srpc.Mux
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	mux := srpc.NewMux()
	ctrl := &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
		mux:  mux,
	}
	_ = mux.Register(bldr_web_plugin_browser.NewSRPCWebPluginBrowserHostHandler(ctrl, ctrl.GetServiceID()))
	return ctrl
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"web plugin browser host controller",
	)
}

// Execute executes the controller.
// Returning nil ends execution.
func (c *Controller) Execute(ctx context.Context) (rerr error) {
	return nil
}

// GetServiceID returns the ServiceID the controller will respond to.
func (c *Controller) GetServiceID() string {
	serviceID := c.conf.GetServiceId()
	if serviceID == "" {
		serviceID = bldr_web_plugin_browser.SRPCWebPluginBrowserHostServiceID
	}
	return serviceID
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		serviceID := d.LookupRpcServiceID()
		if serviceID == c.GetServiceID() || serviceID == web_view.SRPCAccessWebViewsServiceID {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c), nil)
		}
	}
	return nil, nil
}

// InvokeMethod invokes the method matching the service & method ID.
// Returns false, nil if not found.
// If service string is empty, ignore it.
func (c *Controller) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	return c.mux.InvokeMethod(serviceID, methodID, strm)
}

// PluginRpc handles an incoming RPC request for the web plugin from a remote plugin.
func (c *Controller) PluginRpc(strm bldr_web_plugin_browser.SRPCWebPluginBrowserHost_PluginRpcStream) error {
	// Pretend as if the request had come directly from the plugin to the plugin host.
	return rpcstream.HandleRpcStream(strm, func(ctx context.Context, remotePluginID string) (srpc.Invoker, func(), error) {
		return bifrost_rpc.NewInvoker(c.bus, "plugin/"+remotePluginID, true), nil, nil
	})
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller = ((*Controller)(nil))
	_ srpc.Invoker          = ((*Controller)(nil))

	_ bldr_web_plugin_browser.SRPCWebPluginBrowserHostServer = ((*Controller)(nil))
)
