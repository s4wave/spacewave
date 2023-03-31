package bldr_web_plugin_controller

import (
	"context"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	plugin_handle_web_view "github.com/aperturerobotics/bldr/plugin/handle-web-view"
	bldr_web_plugin "github.com/aperturerobotics/bldr/web/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
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
	ctrl := &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
		mux:  mux,
	}
	_ = mux.Register(bldr_web_plugin.NewSRPCWebPluginHandler(ctrl, ctrl.GetServiceID()))
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
		if d.LookupRpcServiceID() == c.GetServiceID() {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c), nil)
		}
	}
	return nil, nil
}

// InvokeMethod invokes the method matching the service & method ID.
// Returns false, nil if not found.
// If service string is empty, ignore it.
func (c *Controller) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	if serviceID != "" && serviceID != c.GetServiceID() {
		return false, nil
	}
	return c.mux.InvokeMethod(serviceID, methodID, strm)
}

// HandleWebViewViaPlugin starts a controller to forward web views to a plugin RPC.
func (c *Controller) HandleWebViewViaPlugin(
	req *bldr_web_plugin.HandleWebViewViaPluginRequest,
	strm bldr_web_plugin.SRPCWebPlugin_HandleWebViewViaPluginStream,
) error {
	conf := &plugin_handle_web_view.Config{
		PluginId:       req.GetHandlePluginId(),
		WebViewIdRegex: req.GetWebViewIdRegex(),
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
