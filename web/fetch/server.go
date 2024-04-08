package web_fetch

import "net/http"

// FetchServer is a srpc server implementing FetchService.
type FetchServer struct {
	handler http.HandlerFunc
}

// NewFetchServer constructs a new FetchServer with a handler.
func NewFetchServer(handler http.HandlerFunc) *FetchServer {
	return &FetchServer{handler: handler}
}

// Fetch executes the fetch request.
func (s *FetchServer) Fetch(strm SRPCFetchService_FetchStream) error {
	return HandleFetch(strm, s.handler)
}

// _ is a type assertion
var _ SRPCFetchServiceServer = ((*FetchServer)(nil))
