package web

import (
	"context"

	"github.com/aperturerobotics/bldr/runtime"
)

// RemoteWebView implements the browser page APIs for the runtime.
type RemoteWebView struct {
	// ctx is the root context
	ctx context.Context
	// r is the remote
	r *Remote
	// id is the identifier for the webview
	id string
	// permanent indicates the web view cannot be closed
	permanent bool
}

// NewRemoteWebView constructs a new remote WebView handle.
//
// if permanent, this web view is the primary and cannot be closed
func NewRemoteWebView(ctx context.Context, r *Remote, id string, permanent bool) *RemoteWebView {
	return &RemoteWebView{ctx: ctx, r: r, id: id, permanent: permanent}
}

// Close shuts down the WebView and closes the window/tab if possible.
// Returns ErrWebViewPermanent if the view cannot be closed.
// Note: browser windows not created by CreateWebView cannot be closed.
func (w *RemoteWebView) Close() error {
	if w.permanent {
		return runtime.ErrWebViewPermanent
	}

	// TODO
	return nil
}

// writeQueryViewStatus writes the query view status command.
func (w *RemoteWebView) writeQueryViewStatus() error {
	msg := NewQueryViewStatus()
	return w.r.WriteMessage(msg)
}

// closeWindow is the internal implementation of Close.
func (w *RemoteWebView) closeWindow() {
	if !w.permanent {
		// TODO
	}
}

// _ is a type assertion
var _ runtime.WebView = ((*RemoteWebView)(nil))
