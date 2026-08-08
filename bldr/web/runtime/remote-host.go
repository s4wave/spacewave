package web_runtime

import (
	"context"
	"os"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/sirupsen/logrus"
)

// remoteWebRuntimeHost implements the WebRuntimeHost RPC service with the Remote.
type remoteWebRuntimeHost struct {
	r *Remote
}

// newRemoteWebRuntimeHost builds the WebRuntimeHost bound to the Remote.
func newRemoteWebRuntimeHost(r *Remote) *remoteWebRuntimeHost {
	return &remoteWebRuntimeHost{r: r}
}

// RequestRuntimeQuit asks the host process to follow the same shutdown path as
// an interactive interrupt.
func (r *remoteWebRuntimeHost) RequestRuntimeQuit(
	context.Context,
	*RequestRuntimeQuitRequest,
) (*RequestRuntimeQuitResponse, error) {
	go signalCurrentProcessInterrupt(r.r.le)
	return &RequestRuntimeQuitResponse{}, nil
}

// WebDocumentRpc opens a stream for a RPC call for a WebDocument.
func (r *remoteWebRuntimeHost) WebDocumentRpc(stream SRPCWebRuntimeHost_WebDocumentRpcStream) error {
	return rpcstream.HandleRpcStream(stream, r.r.GetWebDocumentHost)
}

// WebWorkerRpc opens a stream for a RPC call for a WebWorker.
func (r *remoteWebRuntimeHost) WebWorkerRpc(stream SRPCWebRuntimeHost_WebWorkerRpcStream) error {
	return rpcstream.HandleRpcStream(stream, r.r.GetWebWorkerHost)
}

// ServiceWorkerRpc opens a stream for a RPC call for a ServiceWorker.
func (r *remoteWebRuntimeHost) ServiceWorkerRpc(stream SRPCWebRuntimeHost_ServiceWorkerRpcStream) error {
	return rpcstream.HandleRpcStream(stream, r.r.GetServiceWorkerHost)
}

// _ is a type assertion
var _ SRPCWebRuntimeHostServer = (*remoteWebRuntimeHost)(nil)

func signalCurrentProcessInterrupt(le *logrus.Entry) {
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		le.WithError(err).Warn("failed to find host process")
		return
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		le.WithError(err).Warn("failed to interrupt host process")
		if err := proc.Signal(os.Kill); err != nil {
			le.WithError(err).Warn("failed to terminate host process")
		}
	}
}
