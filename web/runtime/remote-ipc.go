package web_runtime

import "errors"

// remoteHostRuntime implements the HostRuntime RPC service with the Remote.
type remoteHostRuntime struct {
	r *Remote
}

// newRemoteHostRuntime builds the HostRuntime bound to the Remote.
func newRemoteHostRuntime(r *Remote) *remoteHostRuntime {
	return &remoteHostRuntime{r: r}
}

// WebViewRpc opens a stream for a RPC call for a WebView.
func (r *remoteHostRuntime) WebViewRpc(strm SRPCHostRuntime_WebViewRpcStream) error {
	return errors.New("TODO WebViewRPC")
}

// _ is a type assertion
var _ SRPCHostRuntimeServer = ((*remoteHostRuntime)(nil))
