package web_document

import (
	"github.com/aperturerobotics/starpc/rpcstream"
)

// remoteWebDocumentHost implements the WebDocumentHost RPC service with the Remote.
type remoteWebDocumentHost struct {
	r *Remote
}

// newRemoteWebDocumentHost builds the WebDocumentHost bound to the Remote.
func newRemoteWebDocumentHost(r *Remote) *remoteWebDocumentHost {
	return &remoteWebDocumentHost{r: r}
}

// WebViewRpc opens a stream for a RPC call for a WebView.
func (r *remoteWebDocumentHost) WebViewRpc(stream SRPCWebDocumentHost_WebViewRpcStream) error {
	return rpcstream.HandleRpcStream(stream, r.r.GetWebViewHost)
}

// _ is a type assertion
var _ SRPCWebDocumentHostServer = ((*remoteWebDocumentHost)(nil))
