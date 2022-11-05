package web_view_handler

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/view"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/sirupsen/logrus"
)

// Controller is the web view handler controller.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// info is the controller info
	info *controller.Info
	// webViewId is the web view id to match.
	webViewId string
	// handler is the web view handler
	handler WebViewHandler
}

// NewController constructs a new peer controller.
// If privKey is nil, one will be generated.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	info *controller.Info,
	webViewId string,
	handler WebViewHandler,
) *Controller {
	return &Controller{
		le:        le,
		bus:       bus,
		info:      info,
		webViewId: webViewId,
		handler:   handler,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// Execute executes the given controller.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case web_view.HandleWebView:
		return c.resolveHandleWebView(ctx, di, d)
	}
	return nil, nil
}

// resolveHandleWebView conditionally returns a resolver for a HandleWebView directive.
func (c *Controller) resolveHandleWebView(
	ctx context.Context,
	di directive.Instance,
	d web_view.HandleWebView,
) ([]directive.Resolver, error) {
	webView := d.HandleWebView()
	if webView.GetId() != c.webViewId {
		return nil, nil
	}

	return directive.R(NewHandleWebViewResolverWithRetry(c.le, d, c.handler), nil)
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
