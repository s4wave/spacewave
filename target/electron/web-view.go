package electron

import (
	"context"

	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
)

// WebView implements the Electron WebView for the runtime.
type WebView struct {
}

// NewWebView constructs a new WebView handle.
func NewWebView() *WebView {
	return &WebView{}
}

// Remove shuts down the WebView and closes the window/tab if possible.
// Returns ErrWebViewPermanent if the view cannot be closed.
// Returns context.Canceled if ctx is canceled (but still processes the op)
// Note: browser windows not created by CreateWebView cannot be closed.
func (w *WebView) Remove(ctx context.Context) error {
	// TODO
	return web_runtime.ErrWebViewPermanent
}

// _ is a type assertion
var _ web_runtime.WebView = ((*WebView)(nil))
