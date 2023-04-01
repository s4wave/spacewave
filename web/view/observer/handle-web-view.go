package bldr_web_view_observer

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/view"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// observeHandleWebView observes the HandleWebView directive.
func (c *Controller) observeHandleWebView(
	ctx context.Context,
	di directive.Instance,
	dir web_view.HandleWebView,
) error {
	webView := dir.HandleWebView()
	if webView == nil {
		return nil
	}
	webViewID := webView.GetId()
	c.mtx.Lock()
	if c.webViews[webViewID] != webView {
		c.webViews[webViewID] = webView
		c.bcast.Broadcast()
	}
	c.mtx.Unlock()

	// remove the web view when the directive is disposed
	_ = di.AddReference(bus.NewCallbackHandler(nil, nil, func() {
		c.mtx.Lock()
		if c.webViews[webViewID] == webView {
			delete(c.webViews, webViewID)
			c.bcast.Broadcast()
		}
		c.mtx.Unlock()
	}), true)

	return nil
}
