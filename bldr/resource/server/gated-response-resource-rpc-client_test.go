package resource_server_test

import "github.com/s4wave/spacewave/bldr/resource"

// gatedResponseResourceRPCClient discards response data after caller cancellation.
type gatedResponseResourceRPCClient struct {
	// SRPCResourceService_ResourceRpcClient carries the real route and data frames.
	resource.SRPCResourceService_ResourceRpcClient
	// received reports that a response reached the transport before decoding.
	received chan<- struct{}
}

// Recv forwards the route acknowledgement and holds data until the caller cancels.
func (s *gatedResponseResourceRPCClient) Recv() (*resource.ResourceRpcPacket, error) {
	// Receive an actual server packet before withholding its response payload.
	packet, err := s.SRPCResourceService_ResourceRpcClient.Recv()
	if err != nil {
		return nil, err
	}
	if _, ok := packet.GetBody().(*resource.ResourceRpcPacket_Data); !ok {
		return packet, nil
	}

	// Report delivery at the transport while leaving the resource reply unread.
	s.received <- struct{}{}
	<-s.Context().Done()
	return nil, s.Context().Err()
}

// _ is a type assertion.
var _ resource.SRPCResourceService_ResourceRpcClient = (*gatedResponseResourceRPCClient)(nil)
