package bldr_web_plugin_controller

import (
	"context"
	"path"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_forward_rpc_service "github.com/aperturerobotics/bldr/plugin/forward-rpc-service"
	plugin_handle_web_view "github.com/aperturerobotics/bldr/plugin/handle-web-view"
	web_pkg_rpc "github.com/aperturerobotics/bldr/web/pkg/rpc"
	web_pkg_rpc_client "github.com/aperturerobotics/bldr/web/pkg/rpc/client"
	bldr_web_plugin "github.com/aperturerobotics/bldr/web/plugin"
	web_view "github.com/aperturerobotics/bldr/web/view"
	web_view_server "github.com/aperturerobotics/bldr/web/view/server"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/web/plugin/controller"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller manages running the web plugin.
// Serves the WebPlugin RPC service.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// mux is the rpc mux for the WebPlugin RPC service.
	mux srpc.Mux
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	mux := srpc.NewMux()
	// mux = srpc.NewVMux(mux, le, true) // TODO
	ctrl := &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
		mux:  mux,
	}
	_ = mux.Register(bldr_web_plugin.NewSRPCWebPluginHandler(ctrl, ctrl.GetServiceID()))
	_ = web_view.SRPCRegisterAccessWebViews(mux, web_view_server.NewAccessWebViewsViaBus(le, bus))
	return ctrl
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"web plugin controller",
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
		serviceID = bldr_web_plugin.SRPCWebPluginServiceID
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

// HandleWebViewViaPlugin starts a controller to forward web views to a plugin RPC.
func (c *Controller) HandleWebViewViaPlugin(
	req *bldr_web_plugin.HandleWebViewViaPluginRequest,
	strm bldr_web_plugin.SRPCWebPlugin_HandleWebViewViaPluginStream,
) error {
	conf := &plugin_handle_web_view.Config{
		PluginId:    req.GetHandlePluginId(),
		WebViewIdRe: req.GetWebViewIdRe(),
	}
	if err := conf.Validate(); err != nil {
		return err
	}

	ctrl := plugin_handle_web_view.NewController(c.le, c.bus, conf)
	exitErrCh := make(chan error, 1)
	relCtrl, err := c.bus.AddController(strm.Context(), ctrl, func(exitErr error) {
		exitErrCh <- exitErr
	})
	if err != nil {
		return err
	}
	defer relCtrl()

	if err := strm.Send(&bldr_web_plugin.HandleWebViewViaPluginResponse{
		Body: &bldr_web_plugin.HandleWebViewViaPluginResponse_Ready{Ready: true},
	}); err != nil {
		return err
	}

	select {
	case <-strm.Context().Done():
		return context.Canceled
	case err := <-exitErrCh:
		return err
	}
}

// HandleWebPkgViaPlugin starts a controller to forward web pkgs to a plugin RPC.
func (c *Controller) HandleWebPkgViaPlugin(
	req *bldr_web_plugin.HandleWebPkgViaPluginRequest,
	strm bldr_web_plugin.SRPCWebPlugin_HandleWebPkgViaPluginStream,
) error {
	if err := bldr_plugin.ValidatePluginID(req.GetHandlePluginId(), false); err != nil {
		return err
	}
	conf := &web_pkg_rpc_client.Config{
		ServiceIdPrefix: path.Join(
			bldr_plugin.PluginServiceIDPrefix,
			req.GetHandlePluginId(),
			web_pkg_rpc.SRPCAccessWebPkgServiceID,
		),
		ClientId:         "bldr/web/plugin",
		WebPkgIdRe:       req.GetWebPkgIdRe(),
		WebPkgIdPrefixes: req.GetWebPkgIdPrefixes(),
		WebPkgIdList:     req.GetWebPkgIdList(),
	}
	if err := conf.Validate(); err != nil {
		return err
	}

	ctrl, err := web_pkg_rpc_client.NewController(c.le, c.bus, conf)
	if err != nil {
		return err
	}

	exitErrCh := make(chan error, 1)
	relCtrl, err := c.bus.AddController(strm.Context(), ctrl, func(exitErr error) {
		exitErrCh <- exitErr
	})
	if err != nil {
		return err
	}
	defer relCtrl()

	if err := strm.Send(&bldr_web_plugin.HandleWebPkgViaPluginResponse{
		Body: &bldr_web_plugin.HandleWebPkgViaPluginResponse_Ready{Ready: true},
	}); err != nil {
		return err
	}

	select {
	case <-strm.Context().Done():
		return context.Canceled
	case err := <-exitErrCh:
		return err
	}
}

// HandleRpcViaPlugin starts a controller to forward rpcs to a plugin.
func (c *Controller) HandleRpcViaPlugin(
	req *bldr_web_plugin.HandleRpcViaPluginRequest,
	strm bldr_web_plugin.SRPCWebPlugin_HandleRpcViaPluginStream,
) error {
	conf := &plugin_forward_rpc_service.Config{
		PluginId:    req.GetHandlePluginId(),
		ServiceIdRe: req.GetServiceIdRe(),
		ServerIdRe:  req.GetServerIdRe(),
		Backoff:     req.GetBackoff(),
	}
	if err := conf.Validate(); err != nil {
		return err
	}

	ctrl := plugin_forward_rpc_service.NewController(c.le, c.bus, conf)
	exitErrCh := make(chan error, 1)
	relCtrl, err := c.bus.AddController(strm.Context(), ctrl, func(exitErr error) {
		exitErrCh <- exitErr
	})
	if err != nil {
		return err
	}
	defer relCtrl()

	if err := strm.Send(&bldr_web_plugin.HandleRpcViaPluginResponse{
		Body: &bldr_web_plugin.HandleRpcViaPluginResponse_Ready{Ready: true},
	}); err != nil {
		return err
	}

	select {
	case <-strm.Context().Done():
		return context.Canceled
	case err := <-exitErrCh:
		return err
	}
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller               = ((*Controller)(nil))
	_ bldr_web_plugin.SRPCWebPluginServer = ((*Controller)(nil))
	_ srpc.Invoker                        = ((*Controller)(nil))
)
