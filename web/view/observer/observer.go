package bldr_web_view_observer

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/view"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/blang/semver"
)

// ControllerID is the controller id.
const ControllerID = "bldr/web/view/observer"

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "resolves LookupWebView"

// Controller is the web view observer controller.
// Resolves LookupWebView with values from HandleWebView.
// Observes HandleWebView directives.
type Controller struct {
	*bus.BusController[*Config]

	// bcast guards below fields
	bcast broadcast.Broadcast
	// webViews is the set of observed web views
	webViews map[string]web_view.WebView
}

// NewFactory constructs the factory.
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
			return &Controller{
				BusController: base,
				webViews:      make(map[string]web_view.WebView),
			}, nil
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
	case web_view.HandleWebView:
		return nil, c.observeHandleWebView(ctx, di, dir)
	case web_view.LookupWebView:
		return c.resolveLookupWebView(ctx, di, dir)
	}

	return nil, nil
}

// LookupWebView looks up a web view on the observer.
// if !wait, nil, nil is returned if the web view was not found.
func (c *Controller) LookupWebView(ctx context.Context, webViewID string, wait bool) (web_view.WebView, error) {
	for {
		var waitCh <-chan struct{}
		var webView web_view.WebView
		var ok bool
		c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			webView, ok = c.webViews[webViewID]
			if !ok && wait {
				waitCh = getWaitCh()
			}
		})

		if ok || !wait {
			return webView, nil
		}

		select {
		case <-ctx.Done():
			return nil, context.Canceled
		case <-waitCh:
		}
	}
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
