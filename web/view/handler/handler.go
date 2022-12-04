package web_view_handler

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/view"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccall"
	"github.com/sirupsen/logrus"
)

// WebViewHandler handles a WebView.
type WebViewHandler func(
	ctx context.Context,
	webView web_view.WebView,
) error

// MergeWebViewHandlers merges multiple handlers into a single WebViewHandler.
//
// Calls all handlers concurrently, returns first error.
func MergeWebViewHandlers(handlers ...WebViewHandler) WebViewHandler {
	return func(ctx context.Context, webView web_view.WebView) error {
		if len(handlers) == 1 {
			return handlers[0](ctx, webView)
		}

		var ccallFns []ccall.CallConcurrentlyFunc
		for _, handler := range handlers {
			handler := handler
			ccallFns = append(ccallFns, func(ctx context.Context) error {
				return handler(ctx, webView)
			})
		}
		return ccall.CallConcurrently(ctx, ccallFns...)
	}
}

// NewViaBusHandler handles the WebView via the HandleWebView directive.
//
// If returnIfErr is set, returns an error if any of the resolvers fail.
// returnIfErr should be set to true in most cases.
func NewViaBusHandler(le *logrus.Entry, b bus.Bus, returnIfErr bool) WebViewHandler {
	return func(
		ctx context.Context,
		webView web_view.WebView,
	) error {
		return web_view.ExHandleWebView(ctx, le, b, webView, returnIfErr)
	}
}

// NewSetRenderMode builds a new handler that sets the render mode.
//
// le can be nil
func NewSetRenderMode(le *logrus.Entry, req *web_view.SetRenderModeRequest) WebViewHandler {
	return func(
		ctx context.Context,
		webView web_view.WebView,
	) error {
		if le != nil {
			le = req.Logger(le)
			le.Debug("setting render mode")
		}
		_, err := webView.SetRenderMode(ctx, req)
		return err
	}
}

// NewSetReactComponent builds a handler that sets a react component.
//
// le can be empty
func NewSetReactComponent(le *logrus.Entry, scriptPath string) WebViewHandler {
	return NewSetRenderMode(le, &web_view.SetRenderModeRequest{
		// Wait:       true,
		RenderMode: web_view.RenderMode_RenderMode_REACT_COMPONENT,
		ScriptPath: scriptPath,
	})
}

// NewSetFunctionComponent builds a handler that sets a function callback component.
//
// le can be empty
func NewSetFunctionComponent(le *logrus.Entry, scriptPath string) WebViewHandler {
	return NewSetRenderMode(le, &web_view.SetRenderModeRequest{
		// Wait:       true,
		RenderMode: web_view.RenderMode_RenderMode_FUNCTION,
		ScriptPath: scriptPath,
	})
}

// NewSetHtmlLinks builds a new handler that sets html links.
//
// le can be nil
func NewSetHtmlLinks(le *logrus.Entry, req *web_view.SetHtmlLinksRequest) WebViewHandler {
	return func(
		ctx context.Context,
		webView web_view.WebView,
	) error {
		if le != nil {
			le = req.Logger(le)
			le.Debug("setting html links")
		}
		_, err := webView.SetHtmlLinks(ctx, req)
		return err
	}
}
