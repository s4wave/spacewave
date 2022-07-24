package web_runtime_sw

import (
	fetch "github.com/aperturerobotics/bldr/web/fetch"
)

// ServiceWorkerHandler is the handler for the ServiceWorker.
type ServiceWorkerHandler interface {
	// HandleFetch handles an incoming Fetch request from the ServiceWorker.
	// The Client ID can be used to distinguish between windows / browser tabs.
	HandleFetch(strm fetch.SRPCFetchService_FetchStream) error
}

// serviceWorkerHost implements the ServiceWorkerHost RPC service.
type serviceWorkerHost struct {
	handler ServiceWorkerHandler
}

// NewServiceWorkerHost builds the ServiceWorkerHost bound to the Handler.
func NewServiceWorkerHost(handler ServiceWorkerHandler) *serviceWorkerHost {
	return &serviceWorkerHost{handler: handler}
}

// Fetch opens a stream for a ServiceWorker Fetch request.
func (r *serviceWorkerHost) Fetch(strm SRPCServiceWorkerHost_FetchStream) error {
	return r.handler.HandleFetch(strm)
}

// _ is a type assertion
var _ SRPCServiceWorkerHostServer = ((*serviceWorkerHost)(nil))
