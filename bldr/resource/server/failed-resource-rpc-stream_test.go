package resource_server_test

import "github.com/s4wave/spacewave/bldr/resource"

// failedResourceRPCStream carries the real request and refuses response data.
type failedResourceRPCStream struct {
	// SRPCResourceService_ResourceRpcStream carries the route and request frames.
	resource.SRPCResourceService_ResourceRpcStream
	// err is returned when response data would be sent.
	err error
}

// Send accepts the route acknowledgement and refuses subsequent response data.
func (s *failedResourceRPCStream) Send(packet *resource.ResourceRpcPacket) error {
	if _, ok := packet.GetBody().(*resource.ResourceRpcPacket_Data); ok {
		return s.err
	}
	return s.SRPCResourceService_ResourceRpcStream.Send(packet)
}

// _ is a type assertion
var _ resource.SRPCResourceService_ResourceRpcStream = (*failedResourceRPCStream)(nil)
