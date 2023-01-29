package web_document

import (
	view "github.com/aperturerobotics/bldr/web/view"
)

// remoteWebViewHost implements the WebViewHost RPC service with the Remote.
type remoteWebViewHost struct {
	r *Remote
}

// newRemoteWebViewHost builds the WebViewHost bound to the Remote.
func newRemoteWebViewHost(r *Remote) *remoteWebViewHost {
	return &remoteWebViewHost{r: r}
}

// _ is a type assertion
var _ view.SRPCWebViewHostServer = ((*remoteWebViewHost)(nil))
