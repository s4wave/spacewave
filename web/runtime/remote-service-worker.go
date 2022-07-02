package web_runtime

import (
	"context"

	sw "github.com/aperturerobotics/bldr/web/runtime/sw"
)

// remoteServiceWorkerHost implements the ServiceWorkerHost RPC service with the Remote.
type remoteServiceWorkerHost struct {
	r *Remote
}

// newRemoteServiceWorkerHost builds the ServiceWorkerHost bound to the Remote.
func newRemoteServiceWorkerHost(r *Remote) *remoteServiceWorkerHost {
	return &remoteServiceWorkerHost{r: r}
}

// Echo is the echo example.
// TODO: remove this test
func (r *remoteServiceWorkerHost) Echo(ctx context.Context, msg *sw.EchoMsg) (*sw.EchoMsg, error) {
	return msg, nil
}

// _ is a type assertion
var _ sw.SRPCServiceWorkerHostServer = ((*remoteServiceWorkerHost)(nil))
