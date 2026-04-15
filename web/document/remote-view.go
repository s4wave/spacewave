package web_document

import (
	"context"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	web_view "github.com/aperturerobotics/bldr/web/view"
	web_view_client "github.com/aperturerobotics/bldr/web/view/client"
	"github.com/aperturerobotics/starpc/srpc"
)

// remoteWebView contains remote web view information.
type remoteWebView struct {
	ctx            context.Context
	cancel         context.CancelFunc
	proxy          *web_view_client.ProxyWebView
	webViewHostMux srpc.Mux
}

// buildRemoteWebView constructs a new remote WebView handle.
//
// if permanent, this web view is the primary and cannot be closed
func (r *Remote) buildRemoteWebView(rctx context.Context, id, parent, document string, permanent bool) *remoteWebView {
	// #nosec G118 -- cancel func is returned on remoteWebView and invoked on view teardown.
	ctx, ctxCancel := context.WithCancel(rctx)
	client := srpc.NewClient(r.GetWebViewOpenStream(id))
	view := web_view.NewSRPCWebViewClient(client)
	v := web_view_client.NewProxyWebView(ctx, id, parent, document, permanent, client, view)

	// webViewHostMux is used for incoming requests to the web view host mux
	// set true here to wait for services to be available
	busInvoker := bifrost_rpc.NewInvoker(r.bus, web_view.WebViewServerID(id), true)
	webViewHostMux := srpc.NewMux(busInvoker)
	_ = web_view.SRPCRegisterWebViewHost(webViewHostMux, r)

	return &remoteWebView{
		ctx:            ctx,
		cancel:         ctxCancel,
		proxy:          v,
		webViewHostMux: webViewHostMux,
	}
}
