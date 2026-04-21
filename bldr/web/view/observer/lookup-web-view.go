package bldr_web_view_observer

import (
	"context"

	web_view "github.com/s4wave/spacewave/bldr/web/view"
	"github.com/aperturerobotics/controllerbus/directive"
)

// resolveLookupWebView resolves looking up the web view.
func (c *Controller) resolveLookupWebView(
	ctx context.Context,
	di directive.Instance,
	dir web_view.LookupWebView,
) ([]directive.Resolver, error) {
	webViewID := dir.LookupWebViewID()
	if webViewID == "" {
		return nil, nil
	}
	return directive.R(&lookupWebViewResolver{
		c:   c,
		di:  di,
		dir: dir,
	}, nil)
}

// lookupWebViewResolver observes the HandleWebView directive.
type lookupWebViewResolver struct {
	// c is the controller
	c *Controller
	// di is the directive instance
	di directive.Instance
	// dir is the directive
	dir web_view.LookupWebView
}

// Resolve resolves the values, emitting them to the handler.
func (r *lookupWebViewResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	webViewID := r.dir.LookupWebViewID()
	var currValue web_view.LookupWebViewValue
	for {
		var waitCh <-chan struct{}
		var webView web_view.WebView
		r.c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
			webView = r.c.webViews[webViewID]
		})

		if currValue != webView {
			_ = handler.ClearValues()
			currValue = webView
			if currValue != nil {
				_, _ = handler.AddValue(currValue)
				handler.MarkIdle(true)
			}
		}

		select {
		case <-ctx.Done():
			return context.Canceled
		case <-waitCh:
		}
	}
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupWebViewResolver)(nil))
