package web_document

import (
	"context"

	web_view "github.com/aperturerobotics/bldr/web/view"
	web_view_client "github.com/aperturerobotics/bldr/web/view/client"
	"github.com/aperturerobotics/starpc/srpc"
)

// buildRemoteWebView constructs a new remote WebView handle.
//
// if permanent, this web view is the primary and cannot be closed
func (r *Remote) buildRemoteWebView(ctx context.Context, id, parent, document string, permanent bool) *web_view_client.ProxyWebView {
	client := srpc.NewClient(r.GetWebViewOpenStream(id))
	view := web_view.NewSRPCWebViewClient(client)
	v := web_view_client.NewProxyWebView(ctx, id, parent, document, permanent, client, view)
	return v
}
