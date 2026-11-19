package resource_server_test

import (
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
)

// failedResourceRPCStream carries the real request and refuses response data.
type failedResourceRPCStream struct {
	// SRPCResourceService_ResourceRpcStream carries the route and request frames.
	resource.SRPCResourceService_ResourceRpcStream
	// err is returned when response data would be sent.
	err error
}

// MsgSend refuses response data. Route controls are sent with the embedded Send
// and reach the transport.
func (s *failedResourceRPCStream) MsgSend(srpc.Message) error {
	return s.err
}

// _ is a type assertion
var _ resource.SRPCResourceService_ResourceRpcStream = (*failedResourceRPCStream)(nil)
