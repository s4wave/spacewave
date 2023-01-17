package web_document

import (
	"context"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	web_view "github.com/aperturerobotics/bldr/web/view"
	web_view_client "github.com/aperturerobotics/bldr/web/view/client"
	"github.com/aperturerobotics/starpc/srpc"
)

// buildRemoteWebView constructs a new remote WebView handle.
//
// if permanent, this web view is the primary and cannot be closed
func (r *Remote) buildRemoteWebView(ctx context.Context, id, parent, document string, permanent bool) *remoteWebView {
	client := srpc.NewClient(r.GetWebViewOpenStream(id))
	view := web_view.NewSRPCWebViewClient(client)
	v := web_view_client.NewProxyWebView(ctx, id, parent, document, permanent, client, view)

	// webViewHostMux is used for incoming requests to the web view host mux
	busInvoker := bifrost_rpc.NewInvoker(r.bus, web_view.WebViewServerID(id))
	webViewHostMux := srpc.NewMux(busInvoker)
	_ = web_view.SRPCRegisterWebViewHost(webViewHostMux, r)

	return &remoteWebView{
		proxy:          v,
		webViewHostMux: webViewHostMux,
	}
}
