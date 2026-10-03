package bldr_web_plugin_controller

import (
	"context"
	"path"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_forward_rpc_service "github.com/s4wave/spacewave/bldr/plugin/forward-rpc-service"
	plugin_handle_web_view "github.com/s4wave/spacewave/bldr/plugin/handle-web-view"
	web_pkg_fs_controller "github.com/s4wave/spacewave/bldr/web/pkg/fs/controller"
	web_pkg_rpc "github.com/s4wave/spacewave/bldr/web/pkg/rpc"
	web_pkg_rpc_client "github.com/s4wave/spacewave/bldr/web/pkg/rpc/client"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
	web_view "github.com/s4wave/spacewave/bldr/web/view"
	web_view_handler_controller "github.com/s4wave/spacewave/bldr/web/view/handler/controller"
	web_view_server "github.com/s4wave/spacewave/bldr/web/view/server"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/web/plugin/controller"

// Version is the version of this controller.
var Version = controller.MustParseVersion("0.0.1")

// Controller manages running the web plugin.
// Serves the WebPlugin RPC service.
type Controller struct {
	// le is the root logger.
	le *logrus.Entry
	// bus is the controller bus.
	bus bus.Bus
	// conf is the config.
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
	// Construct the plugin-local service registry.
	mux := srpc.NewMux()
	ctrl := &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
		mux:  mux,
	}

	// Register the plugin routing and desktop services on the same RPC boundary.
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

// OpenOrFocusDesktop forwards one desktop request to the plugin's Electron
// controller and returns only after its main process acknowledges the operation.
func (c *Controller) OpenOrFocusDesktop(
	ctx context.Context,
	req *bldr_web_plugin.OpenOrFocusDesktopRequest,
) (*bldr_web_plugin.OpenOrFocusDesktopResponse, error) {
	// Resolve Electron capability without starting a second plugin or runtime.
	desktop, _, ref, err := bldr_web_plugin.ExLookupDesktop(ctx, c.bus)
	if err != nil {
		return nil, err
	}
	if desktop == nil {
		return nil, errors.New("desktop Electron capability unavailable; install a native desktop web plugin artifact")
	}
	defer ref.Release()

	// Return the Electron owner's acknowledgement and shell identity.
	generation, err := desktop.OpenOrFocusMainWindow(ctx, req)
	if err != nil {
		return nil, err
	}
	return &bldr_web_plugin.OpenOrFocusDesktopResponse{Generation: generation}, nil
}

// WatchDesktopPresence reports one Electron shell generation's current state
// and completes after that generation ends, even if a newer shell opens.
func (c *Controller) WatchDesktopPresence(
	req *bldr_web_plugin.WatchDesktopPresenceRequest,
	strm bldr_web_plugin.SRPCWebPlugin_WatchDesktopPresenceStream,
) error {
	// Require the generation returned by a successful open request.
	if req.GetGeneration() == 0 {
		return errors.New("desktop generation is required")
	}

	// Resolve the Electron owner and snapshot that exact shell lifetime.
	desktop, _, ref, err := bldr_web_plugin.ExLookupDesktop(strm.Context(), c.bus)
	if err != nil {
		return err
	}
	if desktop == nil {
		return errors.New("desktop Electron capability unavailable; install a native desktop web plugin artifact")
	}
	defer ref.Release()
	presence := desktop.DesktopPresence(req.GetGeneration())
	if presence == nil {
		return strm.Send(&bldr_web_plugin.WatchDesktopPresenceResponse{
			State: bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED,
		})
	}

	// Forward the owner's current state and completion without inferring an exit.
	var previous *bldr_web_plugin.WatchDesktopPresenceResponse
	for {
		state, err := presence.WaitValueChange(strm.Context(), previous, nil)
		if err != nil {
			return err
		}
		if err := strm.Send(state); err != nil {
			return err
		}
		if state.GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED {
			return nil
		}
		previous = state
	}
}

// WaitDesktopExit waits for the Electron owner's terminal state for one shell
// generation. A late caller receives the retained terminal result.
func (c *Controller) WaitDesktopExit(
	ctx context.Context,
	req *bldr_web_plugin.WatchDesktopPresenceRequest,
) (*bldr_web_plugin.WatchDesktopPresenceResponse, error) {
	// Require the generation acknowledged by the Electron owner.
	if req.GetGeneration() == 0 {
		return nil, errors.New("desktop generation is required")
	}

	// Resolve the owner and keep its generation container alive for this wait.
	desktop, _, ref, err := bldr_web_plugin.ExLookupDesktop(ctx, c.bus)
	if err != nil {
		return nil, err
	}
	if desktop == nil {
		return nil, errors.New("desktop Electron capability unavailable; install a native desktop web plugin artifact")
	}
	defer ref.Release()
	presence := desktop.DesktopPresence(req.GetGeneration())
	if presence == nil {
		return &bldr_web_plugin.WatchDesktopPresenceResponse{State: bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED}, nil
	}

	// Read the retained terminal value, or wait for the owner's state change.
	return presence.WaitValueWithValidator(ctx, func(state *bldr_web_plugin.WatchDesktopPresenceResponse) (bool, error) {
		return state.GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED, nil
	}, nil)
}

// HandleWebViewViaPlugin starts a controller to forward web views to a plugin RPC.
func (c *Controller) HandleWebViewViaPlugin(
	req *bldr_web_plugin.HandleWebViewViaPluginRequest,
	strm bldr_web_plugin.SRPCWebPlugin_HandleWebViewViaPluginStream,
) error {
	// Validate the plugin routing request before constructing its controller.
	if err := req.Validate(); err != nil {
		return err
	}

	// Build and validate the plugin web-view routing configuration.
	conf := &plugin_handle_web_view.Config{
		PluginId:    req.GetHandlePluginId(),
		WebViewIdRe: req.GetWebViewIdRe(),
	}
	if err := conf.Validate(); err != nil {
		return err
	}

	// Attach the web-view routing controller and acknowledge readiness on the stream.
	ctrl := plugin_handle_web_view.NewController(c.le, c.bus, conf)
	sendReady := func() error {
		return strm.Send(&bldr_web_plugin.HandleWebViewViaPluginResponse{
			Body: &bldr_web_plugin.HandleWebViewViaPluginResponse_Ready{Ready: true},
		})
	}
	return c.addControllerSendReadyAndWait(strm.Context(), ctrl, sendReady)
}

// HandleWebPkgViaPlugin starts a controller to forward web pkgs to a plugin RPC.
func (c *Controller) HandleWebPkgViaPlugin(
	req *bldr_web_plugin.HandleWebPkgViaPluginRequest,
	strm bldr_web_plugin.SRPCWebPlugin_HandleWebPkgViaPluginStream,
) error {
	// Validate the plugin routing request before constructing its controller.
	if err := req.Validate(); err != nil {
		return err
	}

	// Build and validate the plugin web-package RPC configuration.
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

	// Construct the plugin web-package RPC controller.
	ctrl, err := web_pkg_rpc_client.NewController(c.le, c.bus, conf)
	if err != nil {
		return err
	}

	// Acknowledge the routing controller and keep it attached until the stream ends.
	sendReady := func() error {
		return strm.Send(&bldr_web_plugin.HandleWebPkgViaPluginResponse{
			Body: &bldr_web_plugin.HandleWebPkgViaPluginResponse_Ready{Ready: true},
		})
	}
	return c.addControllerSendReadyAndWait(strm.Context(), ctrl, sendReady)
}

// HandleRpcViaPlugin starts a controller to forward rpcs to a plugin.
func (c *Controller) HandleRpcViaPlugin(
	req *bldr_web_plugin.HandleRpcViaPluginRequest,
	strm bldr_web_plugin.SRPCWebPlugin_HandleRpcViaPluginStream,
) error {
	// Validate the plugin routing request before constructing its controller.
	if err := req.Validate(); err != nil {
		return err
	}

	// Build and validate the plugin RPC forwarding configuration.
	conf := &plugin_forward_rpc_service.Config{
		PluginId:    req.GetHandlePluginId(),
		ServiceIdRe: req.GetServiceIdRe(),
		ServerIdRe:  req.GetServerIdRe(),
		Backoff:     req.GetBackoff(),
	}
	if err := conf.Validate(); err != nil {
		return err
	}

	// Construct the RPC forwarding controller for the stream lifetime.
	ctx := strm.Context()
	ctrl := plugin_forward_rpc_service.NewController(c.le, c.bus, conf)

	// Acknowledge the routing controller and keep it attached until the stream ends.
	sendReady := func() error {
		return strm.Send(&bldr_web_plugin.HandleRpcViaPluginResponse{
			Body: &bldr_web_plugin.HandleRpcViaPluginResponse_Ready{Ready: true},
		})
	}
	return c.addControllerSendReadyAndWait(ctx, ctrl, sendReady)
}

// HandleWebViewViaHandlers configures web view handlers with filtering.
func (c *Controller) HandleWebViewViaHandlers(
	req *bldr_web_plugin.HandleWebViewViaHandlersRequest,
	strm bldr_web_plugin.SRPCWebPlugin_HandleWebViewViaHandlersStream,
) error {
	// Validate the plugin routing request before constructing its controller.
	if err := req.Validate(); err != nil {
		return err
	}
	c.le.WithField("handlers", len(req.GetConfig().GetHandlers())).
		Debug("handling web view handlers request")

	// Add a new WebViewHandlers controller.
	conf := &web_view_handler_controller.Config{Handlers: req.GetConfig()}
	ctrl, err := web_view_handler_controller.NewControllerWithConfig(c.le, conf)
	if err != nil {
		return err
	}

	// Acknowledge the routing controller and keep it attached until the stream ends.
	sendReady := func() error {
		// Report web-view handler readiness to the plugin client.
		c.le.Debug("sending web view handlers ready")
		return strm.Send(&bldr_web_plugin.HandleWebViewViaHandlersResponse{
			Body: &bldr_web_plugin.HandleWebViewViaHandlersResponse_Ready{Ready: true},
		})
	}
	return c.addControllerSendReadyAndWait(strm.Context(), ctrl, sendReady)
}

// HandleWebPkgsViaPluginAssets configures serving web pkgs via a plugin assets fs.
func (c *Controller) HandleWebPkgsViaPluginAssets(
	req *bldr_web_plugin.HandleWebPkgsViaPluginAssetsRequest,
	strm bldr_web_plugin.SRPCWebPlugin_HandleWebPkgsViaPluginAssetsStream,
) error {
	// Validate the plugin routing request before constructing its controller.
	if err := req.Validate(); err != nil {
		return err
	}

	// Construct the web-package controller backed by the plugin assets filesystem.
	ctrl, err := web_pkg_fs_controller.NewController(c.le, c.bus, &web_pkg_fs_controller.Config{
		UnixfsId:     bldr_plugin.PluginAssetsFsId(req.GetHandlePluginId()),
		UnixfsPrefix: req.GetWebPkgsPath(),
		WebPkgIdList: req.GetWebPkgIdList(),
	})
	if err != nil {
		return err
	}

	// Acknowledge the routing controller and keep it attached until the stream ends.
	sendReady := func() error {
		return strm.Send(&bldr_web_plugin.HandleWebPkgsViaPluginAssetsResponse{
			Body: &bldr_web_plugin.HandleWebPkgsViaPluginAssetsResponse_Ready{Ready: true},
		})
	}
	return c.addControllerSendReadyAndWait(strm.Context(), ctrl, sendReady)
}

// addControllerSendReadyAndWait adds a controller, sends ready, and waits for context cancellation or controller failure.
// Returning nil from Execute means the controller did no active work; the handler remains attached until the stream closes.
// Returns the exit error or context.Canceled if the context was cancelled.
func (c *Controller) addControllerSendReadyAndWait(ctx context.Context, ctrl controller.Controller, sendReady func() error) error {
	// Attach the routing controller and retain its execution result.
	exitErrCh := make(chan error, 1)
	relCtrl, err := c.bus.AddController(ctx, ctrl, func(exitErr error) {
		exitErrCh <- exitErr
	})
	if err != nil {
		return err
	}
	defer relCtrl()

	// Confirm the routing controller is attached before waiting on its lifetime.
	if err := sendReady(); err != nil {
		return err
	}

	// Keep the routing controller attached until cancellation or execution failure.
	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-exitErrCh:
			if err == nil {
				continue
			}
			return err
		}
	}
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller               = (*Controller)(nil)
	_ bldr_web_plugin.SRPCWebPluginServer = (*Controller)(nil)
	_ srpc.Invoker                        = (*Controller)(nil)
)
