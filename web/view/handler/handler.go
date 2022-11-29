package web_view_handler

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/view"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/sirupsen/logrus"
)

// WebViewHandler handles a WebView.
type WebViewHandler func(
	ctx context.Context,
	webView web_view.WebView,
) error

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
func NewSetRenderMode(req *web_view.SetRenderModeRequest, le *logrus.Entry) WebViewHandler {
	return func(
		ctx context.Context,
		webView web_view.WebView,
	) error {
		if le != nil {
			le = le.WithField("render-mode", req.GetRenderMode().String())
			if scriptPath := req.GetScriptPath(); scriptPath != "" {
				le = le.WithField("script-path", scriptPath)
			}
			le.Debug("setting render mode")
		}
		_, err := webView.SetRenderMode(ctx, req)
		return err
	}
}

// NewSetReactComponent builds a handler that sets a react component.
//
// le can be empty
func NewSetReactComponent(scriptPath string, le *logrus.Entry) WebViewHandler {
	return NewSetRenderMode(&web_view.SetRenderModeRequest{
		// Wait:       true,
		RenderMode: web_view.RenderMode_RenderMode_REACT_COMPONENT,
		ScriptPath: scriptPath,
	}, le)
}

// NewSetFunctionComponent builds a handler that sets a function callback component.
//
// le can be empty
func NewSetFunctionComponent(scriptPath string, le *logrus.Entry) WebViewHandler {
	return NewSetRenderMode(&web_view.SetRenderModeRequest{
		// Wait:       true,
		RenderMode: web_view.RenderMode_RenderMode_FUNCTION,
		ScriptPath: scriptPath,
	}, le)
}
