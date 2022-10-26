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
	le *logrus.Entry,
	b bus.Bus,
	webView web_view.WebView,
) error

// NewSetRenderMode builds a new handler that sets the render mode.
func NewSetRenderMode(req *web_view.SetRenderModeRequest) WebViewHandler {
	return func(
		ctx context.Context,
		le *logrus.Entry,
		b bus.Bus,
		webView web_view.WebView,
	) error {
		le.Debugf("setting render mode to %s", req.GetRenderMode().String())
		_, err := webView.SetRenderMode(ctx, req)
		return err
	}
}

// NewSetReactComponent builds a handler that sets a react component.
func NewSetReactComponent(scriptPath string) WebViewHandler {
	return NewSetRenderMode(&web_view.SetRenderModeRequest{
		Wait:       true,
		RenderMode: web_view.RenderMode_RenderMode_REACT_COMPONENT,
		ScriptPath: scriptPath,
	})
}
