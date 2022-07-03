package web_runtime

import (
	"encoding/base64"
	"net/http"

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
	h.r.le.Debug("service worker fetch: %s", req.Url)
	var handler http.HandlerFunc = func(rw http.ResponseWriter, req *http.Request) {
		// TODO: Demo image
		rw.Header().Set("Content-Type", "image/png")
		rw.WriteHeader(200)
		// basic test image
		data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAACAAAAAgEAIAAACsiDHgAAAABGdBTUEAAYagMeiWXwAAAOVJREFUeJzVlsEKgzAQRKfgQX/Lfrf9rfaWHgYDkoYmZpPMehiGReQ91qCPEEIAPi/gmu9kcnN+GD0nM1/O4vNad7cC6850KHCiM5fz7fJwXdEBYPOygV/o7PICeXSmsMA/dKbkGShD51xsAzXo7DIC9ehMAYG76MypZ6ANnfNJG7BAZx8uYIfOHChgjR4F+MfuDx0AtmfnDfREZ+8m0B+9m8Ao9Chg9x0Yi877jTYwA529WWAeerPAbPQoUH8GNNA5r9yAEjp7sYAeerGAKnoUyJ8BbXTOMxvwgM6eCPhBTwS8oTO/5kL+Xge7xOwAAAAASUVORK5CYII=")
		rw.Write([]byte(data))
	}
	return fetch.HandleFetch(req, strm, handler)
}

// _ is a type assertion
var _ sw.SRPCServiceWorkerHostServer = ((*remoteServiceWorkerHost)(nil))
