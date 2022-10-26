package web_document

import (
	view "github.com/aperturerobotics/bldr/web/view"
)

// remoteWebViewHost implements the WebViewHost RPC service with the Remote.
type remoteWebViewHost struct {
	r *RemoteWebView
}

// newRemoteWebViewHost builds the WebViewHost bound to the Remote.
func newRemoteWebViewHost(r *RemoteWebView) *remoteWebViewHost {
	return &remoteWebViewHost{r: r}
}

// TODO: rpc methods for host

// _ is a type assertion
var _ view.SRPCWebViewHostServer = ((*remoteWebViewHost)(nil))
