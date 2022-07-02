package web_runtime

import (
	"context"

	view "github.com/aperturerobotics/bldr/web/runtime/view"
	"github.com/aperturerobotics/starpc/srpc"
)

// RemoteWebView implements the browser page APIs for the runtime.
type RemoteWebView struct {
	// ctx is the root context
	ctx context.Context
	// r is the remote
	r *Remote
	// mux is the mux for incoming WebView RPC calls.
	mux srpc.Mux
	// id is the identifier for the webview
	id string
	// permanent indicates the web view cannot be closed
	permanent bool
	// client is the srpc client for the remote WebViewRenderer.
	client srpc.Client
	// renderer is the RPC service for the WebViewRenderer.
	renderer view.SRPCWebViewRendererClient
}

// NewRemoteWebView constructs a new remote WebView handle.
//
// if permanent, this web view is the primary and cannot be closed
func NewRemoteWebView(ctx context.Context, r *Remote, id string, permanent bool) *RemoteWebView {
	mux := srpc.NewMux()
	client := srpc.NewClient(r.GetWebViewOpenStream(id))
	renderer := view.NewSRPCWebViewRendererClient(client)
	v := &RemoteWebView{
		ctx:       ctx,
		r:         r,
		id:        id,
		mux:       mux,
		permanent: permanent,
		client:    client,
		renderer:  renderer,
	}
	_ = view.SRPCRegisterWebViewHost(mux, newRemoteWebViewHost(v))

	return v
}

// GetMux returns the mux for the WebView services.
func (w *RemoteWebView) GetMux() srpc.Mux {
	return w.mux
}

// Remove shuts down the WebView and closes / removes the window/tab, if possible.
// Returns ErrWebViewPermanent if the view cannot be closed.
// Note: browser windows not created by CreateWebView cannot be closed.
func (w *RemoteWebView) Remove(ctx context.Context) error {
	if w.permanent {
		return ErrWebViewPermanent
	}

	return w.r.RemoveWebView(ctx, w.id)
}

// _ is a type assertion
var _ WebView = ((*RemoteWebView)(nil))
