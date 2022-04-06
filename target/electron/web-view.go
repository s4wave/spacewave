package electron

import web_runtime "github.com/aperturerobotics/bldr/web/runtime"

// WebView implements the Electron WebView for the runtime.
type WebView struct {
}

// NewWebView constructs a new WebView handle.
func NewWebView() *WebView {
	return &WebView{}
}

// Close shuts down the WebView and closes the window/tab if possible.
// Returns ErrWebViewPermanent if the view cannot be closed.
// Note: browser windows not created by CreateWebView cannot be closed.
func (w *WebView) Close() error {
	// TODO
	return nil
}

// _ is a type assertion
var _ web_runtime.WebView = ((*WebView)(nil))
