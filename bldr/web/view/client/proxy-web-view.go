package web_view_client

import (
	"context"

	web_view "github.com/s4wave/spacewave/bldr/web/view"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

// ProxyWebView is a WebView which proxies requests to the RPC client.
type ProxyWebView struct {
	// ctx is the root context
	ctx context.Context
	// id is the identifier for the webview
	id string
	// parent is the identifier for the parent webview (if any)
	parent string
	// document is the identifier for the parent document (if any)
	document string
	// permanent indicates the web view cannot be closed
	permanent bool
	// client is the srpc client for the remote WebView.
	client srpc.Client
	// view is the RPC service for the WebView.
	view web_view.SRPCWebViewClient
}

// NewProxyWebView constructs a new ProxyWebView.
func NewProxyWebView(
	ctx context.Context,
	id,
	parent,
	document string,
	permanent bool,
	client srpc.Client,
	view web_view.SRPCWebViewClient,
) *ProxyWebView {
	return &ProxyWebView{
		ctx:       ctx,
		id:        id,
		parent:    parent,
		document:  document,
		permanent: permanent,
		client:    client,
		view:      view,
	}
}

// NewProxyWebViewViaAccess builds a ProxyWebView which accesses the WebView via
// an AccessWebViews service.
func NewProxyWebViewViaAccess(
	ctx context.Context,
	id,
	parent,
	document string,
	permanent bool,
	accessClient web_view.SRPCAccessWebViewsClient,
) *ProxyWebView {
	client := rpcstream.NewRpcStreamClient(accessClient.WebViewRpc, id, false)
	return NewProxyWebView(
		ctx,
		id,
		parent,
		document,
		permanent,
		client,
		web_view.NewSRPCWebViewClient(client),
	)
}

// GetId returns the web view identifier.
func (v *ProxyWebView) GetId() string {
	return v.id
}

// GetParentId returns the id of the parent web view (if any)
func (v *ProxyWebView) GetParentId() string {
	return v.parent
}

// GetDocumentId returns the id of the parent WebDocument.
// May be empty.
func (v *ProxyWebView) GetDocumentId() string {
	return v.document
}

// GetPermanent returns if the web view is not removable.
func (v *ProxyWebView) GetPermanent() bool {
	return v.permanent
}

// GetClient returns the RPC client for WebView and other services.
func (v *ProxyWebView) GetClient() srpc.Client {
	return v.client
}

// SetRenderMode updates the RenderMode of the WebView.
func (v *ProxyWebView) SetRenderMode(
	ctx context.Context,
	req *web_view.SetRenderModeRequest,
) (*web_view.SetRenderModeResponse, error) {
	return v.view.SetRenderMode(ctx, req)
}

// SetHtmlLinks updates the list of HtmlLink on the WebView.
func (v *ProxyWebView) SetHtmlLinks(
	ctx context.Context,
	req *web_view.SetHtmlLinksRequest,
) (*web_view.SetHtmlLinksResponse, error) {
	return v.view.SetHtmlLinks(ctx, req)
}

// ResetWebView resets the web view to the initial state.
func (v *ProxyWebView) ResetWebView(ctx context.Context) error {
	_, err := v.view.ResetWebView(ctx, &web_view.ResetWebViewRequest{})
	return err
}

// Remove shuts down the WebView and closes the window/tab if possible.
// Returns ErrWebViewPermanent if the view cannot be closed.
// Returns context.Canceled if ctx is canceled (but still processes the op)
// Note: browser windows not created by CreateWebView cannot be closed.
func (v *ProxyWebView) Remove(ctx context.Context) error {
	if v.permanent {
		return web_view.ErrWebViewPermanent
	}

	resp, err := v.view.RemoveWebView(ctx, &web_view.RemoveWebViewRequest{})
	if err == nil && !resp.Removed {
		err = web_view.ErrWebViewPermanent
	}
	return err
}

// _ is a type assertion
var _ web_view.WebView = ((*ProxyWebView)(nil))
