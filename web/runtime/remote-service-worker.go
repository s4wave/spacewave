package web_runtime

import (
	"errors"

	fetch "github.com/aperturerobotics/bldr/web/fetch"
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

// Fetch proxies a Fetch request with a streaming response.
func (h *remoteServiceWorkerHost) Fetch(req *fetch.FetchRequest, strm sw.SRPCServiceWorkerHost_FetchStream) error {
	// TODO
	h.r.le.Infof("DEBUG: got service worker fetch: %s", req.Url)
	return errors.New("TODO remote-service-worker fetch")
}

// _ is a type assertion
var _ sw.SRPCServiceWorkerHostServer = ((*remoteServiceWorkerHost)(nil))
