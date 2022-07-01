package web_runtime

import (
	"github.com/aperturerobotics/starpc/rpcstream"
)

// remoteHostRuntime implements the HostRuntime RPC service with the Remote.
type remoteHostRuntime struct {
	r *Remote
}

// newRemoteHostRuntime builds the HostRuntime bound to the Remote.
func newRemoteHostRuntime(r *Remote) *remoteHostRuntime {
	return &remoteHostRuntime{r: r}
}

// WebRuntimeRpc opens a stream for a RPC call for a WebRuntime.
func (r *remoteHostRuntime) WebRuntimeRpc(stream SRPCHostRuntime_WebRuntimeRpcStream) error {
	return rpcstream.HandleRpcStream(stream, r.r.GetWebRuntimeMux)
}

// ServiceWorkerRpc opens a stream for a RPC call for a ServiceWorker.
func (r *remoteHostRuntime) ServiceWorkerRpc(stream SRPCHostRuntime_ServiceWorkerRpcStream) error {
	return rpcstream.HandleRpcStream(stream, r.r.GetServiceWorkerMux)
}

// WebViewRpc opens a stream for a RPC call for a WebView.
func (r *remoteHostRuntime) WebViewRpc(stream SRPCHostRuntime_WebViewRpcStream) error {
	return rpcstream.HandleRpcStream(stream, r.r.GetWebViewMux)
}

// _ is a type assertion
var _ SRPCHostRuntimeServer = ((*remoteHostRuntime)(nil))
