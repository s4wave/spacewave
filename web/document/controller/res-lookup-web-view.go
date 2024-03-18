package web_document_controller

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/view"
	"github.com/aperturerobotics/controllerbus/directive"
)

// resolveLookupWebView resolves a LookupWebView directive.
func (c *Controller) resolveLookupWebView(
	_ context.Context,
	_ directive.Instance,
	dir web_view.LookupWebView,
) ([]directive.Resolver, error) {
	return directive.R(&lookupWebViewResolver{c: c, d: dir}, nil)
}

// lookupWebViewResolver resolves LookupWebView with the controller.
type lookupWebViewResolver struct {
	// c is the controller
	c *Controller
	// d is the directive
	d web_view.LookupWebView
}

// Resolve resolves the values, emitting them to the handler.
func (r *lookupWebViewResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	wv, err := r.c.GetWebDocument().GetWebView(ctx, r.d.LookupWebViewID(), r.d.LookupWebViewWait())
	if err != nil || wv == nil {
		return err
	}
	_, _ = handler.AddValue(wv)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupWebViewResolver)(nil))
