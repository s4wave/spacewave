package main

import "github.com/aperturerobotics/bldr/runtime"

// WebView implements the browser page APIs for the runtime.
type WebView struct {
	// root indicates if this is the root webview (cannot be closed)
	root bool
}

// NewWebView constructs a new WebView handle.
//
// if isRoot, this web view is the primary and cannot be closed
func NewWebView(isRoot bool) *WebView {
	return &WebView{root: isRoot}
}

// Close shuts down the WebView and closes the window/tab if possible.
// Returns ErrWebViewPermanent if the view cannot be closed.
// Note: browser windows not created by CreateWebView cannot be closed.
func (w *WebView) Close() error {
	if w.root {
		return runtime.ErrWebViewPermanent
	}

	// TODO
	return nil
}

// _ is a type assertion
var _ runtime.WebView = ((*WebView)(nil))
