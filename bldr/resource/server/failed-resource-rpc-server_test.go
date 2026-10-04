package resource_server_test

import (
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// failedResourceRPCServer refuses response data after a successful route handshake.
type failedResourceRPCServer struct {
	// ResourceServer supplies the real generation and routing lifecycle.
	*resource_server.ResourceServer
	// err is the response transport error.
	err error
}

// ResourceRpc routes the call over a transport that refuses response data.
func (s *failedResourceRPCServer) ResourceRpc(strm resource.SRPCResourceService_ResourceRpcStream) error {
	return s.ResourceServer.ResourceRpc(&failedResourceRPCStream{SRPCResourceService_ResourceRpcStream: strm, err: s.err})
}

// _ is a type assertion
var _ resource.SRPCResourceServiceServer = (*failedResourceRPCServer)(nil)
