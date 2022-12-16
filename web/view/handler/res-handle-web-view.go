package web_view_handler

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/view"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/backoff"
	"github.com/sirupsen/logrus"
)

// HandleWebViewResolver resolves HandleWebView with a WebViewHandler.
type HandleWebViewResolver struct {
	dir     web_view.HandleWebView
	handler WebViewHandler
}

// NewHandleWebViewResolver returns a new HandleWebViewResolver.
func NewHandleWebViewResolver(
	dir web_view.HandleWebView,
	handler WebViewHandler,
) *HandleWebViewResolver {
	if handler == nil || dir == nil || dir.HandleWebView() == nil {
		return nil
	}
	return &HandleWebViewResolver{dir: dir, handler: handler}
}

// NewHandleWebViewResolverWithRetry builds the HandleWebViewResolver with a RetryResolver.
func NewHandleWebViewResolverWithRetry(le *logrus.Entry, dir web_view.HandleWebView, handler WebViewHandler) *directive.RetryResolver {
	handleResolver := NewHandleWebViewResolver(dir, handler)
	if handleResolver == nil {
		return nil
	}
	retryBackoff := &backoff.Backoff{
		BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
		Exponential: &backoff.Exponential{
			InitialInterval: 100,
			MaxInterval:     4200,
		},
	}
	return directive.NewRetryResolver(le, handleResolver, retryBackoff.Construct())
}

// Resolve resolves the values, emitting them to the handler.
func (r *HandleWebViewResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	return r.handler(ctx, r.dir.HandleWebView())
}

// _ is a type assertion
var _ directive.Resolver = ((*HandleWebViewResolver)(nil))
