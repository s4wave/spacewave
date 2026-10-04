package resource_server_test

import (
	"context"

	"github.com/s4wave/spacewave/bldr/resource"
)

// gatedResponseResourceService holds response data before the resource decoder.
type gatedResponseResourceService struct {
	// SRPCResourceServiceClient supplies the real control and attachment streams.
	resource.SRPCResourceServiceClient
	// received reports that response data reached the client transport.
	received chan<- struct{}
}

// ResourceRpc opens a real route whose response waits for caller cancellation.
func (s *gatedResponseResourceService) ResourceRpc(ctx context.Context) (resource.SRPCResourceService_ResourceRpcClient, error) {
	// Open the route before adding the response-delivery gate.
	stream, err := s.SRPCResourceServiceClient.ResourceRpc(ctx)
	if err != nil {
		return nil, err
	}

	return &gatedResponseResourceRPCClient{
		SRPCResourceService_ResourceRpcClient: stream,
		received:                              s.received,
	}, nil
}

// _ is a type assertion.
var _ resource.SRPCResourceServiceClient = (*gatedResponseResourceService)(nil)
