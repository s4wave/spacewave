package bldr_plugin_handle_web_view

import (
	"context"
	"regexp"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	web_view "github.com/aperturerobotics/bldr/web/view"
	web_view_handler "github.com/aperturerobotics/bldr/web/view/handler"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/plugin/handle-web-view"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller handles web views by loading a plugin and calling a RPC service.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// webViewIdRe is the parsed regex to filter requests by.
	// if nil, accepts any
	webViewIdRe *regexp.Regexp
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	// note: checked in Validate()
	webViewIdRe, _ := conf.ParseWebViewIdRe()
	return &Controller{
		le:          le,
		bus:         bus,
		conf:        conf,
		webViewIdRe: webViewIdRe,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"handles web views via plugin: "+c.conf.GetPluginId(),
	)
}

// Execute executes the controller.
// Returning nil ends execution.
func (c *Controller) Execute(rctx context.Context) (rerr error) {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case web_view.HandleWebView:
		return c.resolveHandleWebView(inst, d)
	}
	return nil, nil
}

// resolveHandleWebView resolves the HandleWebView directive.
func (c *Controller) resolveHandleWebView(di directive.Instance, dir web_view.HandleWebView) ([]directive.Resolver, error) {
	if webViewIdRe := c.webViewIdRe; webViewIdRe != nil {
		webViewID := dir.HandleWebView().GetId()
		if !webViewIdRe.MatchString(webViewID) {
			return nil, nil
		}
	}
	return directive.R(web_view_handler.NewHandleWebViewResolverWithRetry(
		c.le,
		dir,
		c.HandleWebView,
	), nil)
}

// HandleWebView loads the configured plugin and uses its RPC service to handle the view.
// Waits for the plugin to be loaded or ctx to be canceled.
func (c *Controller) HandleWebView(
	ctx context.Context,
	webView web_view.WebView,
) error {
	return bldr_plugin.ExPluginLoadAccessClient(
		ctx,
		c.bus,
		c.conf.GetPluginId(),
		func(ctx context.Context, client srpc.Client) error {
			// fetch via the RPC client
			c.le.Debugf("handling web view %s via plugin %s", webView.GetId(), c.conf.GetPluginId())
			defer c.le.Debugf("exited handling web view %s via plugin %s", webView.GetId(), c.conf.GetPluginId())

			handleViewClient := web_view_handler.NewSRPCHandleWebViewServiceClient(client)
			return web_view_handler.HandleWebViewViaClient(ctx, handleViewClient, webView)
		},
	)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
