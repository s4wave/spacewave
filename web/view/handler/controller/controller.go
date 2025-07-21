package web_view_handler_controller

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/view"
	web_view_handler "github.com/aperturerobotics/bldr/web/view/handler"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller id.
const ControllerID = "bldr/web/view/handler"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller is the web view handler controller.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// handlers is the handlers set
	handlers *web_view_handler.WebViewHandlersWithFilters
}

// NewController constructs a new controller.
func NewController(le *logrus.Entry, handlers *web_view_handler.WebViewHandlersWithFilters) *Controller {
	return &Controller{le: le, handlers: handlers}
}

// NewControllerWithConfig constructs a new controller from a Config.
func NewControllerWithConfig(le *logrus.Entry, conf *Config) (*Controller, error) {
	handlers, err := web_view_handler.NewWebViewHandlersFromConfig(le, conf.GetHandlers())
	if err != nil {
		return nil, err
	}
	return NewController(le, handlers), nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "devtool web entrypoint")
}

// Execute executes the controller goroutine.
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
	webViewID := webView.GetId()
	webViewParentID := webView.GetParentId()

	handlers := c.handlers.GetMatchingHandlers(webViewID, webViewParentID)
	if len(handlers) == 0 {
		return nil, nil
	}

	return directive.R(
		web_view_handler.NewHandleWebViewResolverWithRetry(
			c.le,
			d,
			web_view_handler.MergeWebViewHandlers(handlers...),
		),
		nil,
	)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
