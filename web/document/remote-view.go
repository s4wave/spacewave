package web_document

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/document/view"
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
	// client is the srpc client for the remote WebView.
	client srpc.Client
	// view is the RPC service for the WebView.
	view web_view.SRPCWebViewClient
}

// NewRemoteWebView constructs a new remote WebView handle.
//
// if permanent, this web view is the primary and cannot be closed
func NewRemoteWebView(ctx context.Context, r *Remote, id string, permanent bool) *RemoteWebView {
	mux := srpc.NewMux()
	client := srpc.NewClient(r.GetWebViewOpenStream(id))
	view := web_view.NewSRPCWebViewClient(client)
	v := &RemoteWebView{
		ctx:       ctx,
		r:         r,
		id:        id,
		mux:       mux,
		permanent: permanent,
		client:    client,
		view:      view,
	}
	_ = web_view.SRPCRegisterWebViewHost(mux, newRemoteWebViewHost(v))

	return v
}

// GetWebViewUuid returns the web view identifier.
func (w *RemoteWebView) GetWebViewUuid() string {
	return w.id
}

// GetMux returns the mux for the WebView services.
func (w *RemoteWebView) GetMux() srpc.Mux {
	return w.mux
}

// SetRenderMode updates the RenderMode parameters of the RemoteWebView.
func (w *RemoteWebView) SetRenderMode(
	ctx context.Context,
	in *web_view.SetRenderModeRequest,
) (*web_view.SetRenderModeResponse, error) {
	return w.view.SetRenderMode(ctx, in)
}

// Remove shuts down the WebView and closes / removes the window/tab, if possible.
// Returns ErrWebViewPermanent if the view cannot be closed.
// Note: browser windows not created by CreateWebView cannot be closed.
func (w *RemoteWebView) Remove(ctx context.Context) error {
	if w.permanent {
		return ErrWebViewPermanent
	}

	_, err := w.r.RemoveWebView(ctx, w.id)
	return err
}

// _ is a type assertion
var _ web_view.WebView = ((*RemoteWebView)(nil))
