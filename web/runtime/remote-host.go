package web_runtime

import (
	"github.com/aperturerobotics/starpc/rpcstream"
)

// remoteWebRuntimeHost implements the WebRuntimeHost RPC service with the Remote.
type remoteWebRuntimeHost struct {
	r *Remote
}

// newRemoteWebRuntimeHost builds the WebRuntimeHost bound to the Remote.
func newRemoteWebRuntimeHost(r *Remote) *remoteWebRuntimeHost {
	return &remoteWebRuntimeHost{r: r}
}

// WebDocumentRpc opens a stream for a RPC call for a WebDocument.
func (r *remoteWebRuntimeHost) WebDocumentRpc(stream SRPCWebRuntimeHost_WebDocumentRpcStream) error {
	return rpcstream.HandleRpcStream(stream, r.r.GetWebDocumentHost)
}

// ServiceWorkerRpc opens a stream for a RPC call for a ServiceWorker.
func (r *remoteWebRuntimeHost) ServiceWorkerRpc(stream SRPCWebRuntimeHost_ServiceWorkerRpcStream) error {
	return rpcstream.HandleRpcStream(stream, r.r.GetServiceWorkerHost)
}

// _ is a type assertion
var _ SRPCWebRuntimeHostServer = ((*remoteWebRuntimeHost)(nil))
