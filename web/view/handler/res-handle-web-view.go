package web_view_handler

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/view"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/sirupsen/logrus"
)

// HandleWebViewResolver resolves HandleWebView with a WebViewHandler.
type HandleWebViewResolver struct {
	le      *logrus.Entry
	b       bus.Bus
	dir     web_view.HandleWebView
	handler WebViewHandler
}

// NewHandleWebViewResolver returns a new HandleWebViewResolver.
func NewHandleWebViewResolver(
	le *logrus.Entry,
	b bus.Bus,
	dir web_view.HandleWebView,
	handler WebViewHandler,
) *HandleWebViewResolver {
	if handler == nil || dir == nil || dir.HandleWebView() == nil {
		return nil
	}
	return &HandleWebViewResolver{le: le, b: b, dir: dir, handler: handler}
}

// Resolve resolves the values, emitting them to the handler.
func (r *HandleWebViewResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	return r.handler(ctx, r.le, r.b, r.dir.HandleWebView())
}

// _ is a type assertion
var _ directive.Resolver = ((*HandleWebViewResolver)(nil))
