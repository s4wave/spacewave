package bifrost_api

import (
	bus_api "github.com/aperturerobotics/controllerbus/bus/api"
	"github.com/aperturerobotics/starpc/srpc"
	peer_api "github.com/s4wave/spacewave/net/peer/api"
	pubsub_api "github.com/s4wave/spacewave/net/pubsub/api"
	stream_api "github.com/s4wave/spacewave/net/stream/api"
)

// BifrostAPIServer is the bifrost daemon server interface.
type BifrostAPIServer interface {
	bus_api.SRPCControllerBusServiceServer
	stream_api.SRPCStreamServiceServer
	peer_api.SRPCPeerServiceServer
	pubsub_api.SRPCPubSubServiceServer
}

// SRPCBifrostAPIServer is the bifrost daemon server interface.
type SRPCBifrostAPIServer = BifrostAPIServer

// RegisterAsSRPCServer registers a server with a SRPC mux.
func RegisterAsSRPCServer(s BifrostAPIServer, mux srpc.Mux) {
	_ = bus_api.SRPCRegisterControllerBusService(mux, s)
	_ = stream_api.SRPCRegisterStreamService(mux, s)
	_ = peer_api.SRPCRegisterPeerService(mux, s)
	_ = pubsub_api.SRPCRegisterPubSubService(mux, s)
}

// BifrostAPIClient is the bifrost daemon client interface.
type BifrostAPIClient interface {
	bus_api.SRPCControllerBusServiceClient
	stream_api.SRPCStreamServiceClient
	peer_api.SRPCPeerServiceClient
	pubsub_api.SRPCPubSubServiceClient
}

// SRPCBifrostAPIClient is the bifrost daemon client interface.
type SRPCBifrostAPIClient = BifrostAPIClient

// bifrostAPIClient implements BifrostAPIClient.
type bifrostAPIClient struct {
	bus_api.SRPCControllerBusServiceClient
	stream_api.SRPCStreamServiceClient
	peer_api.SRPCPeerServiceClient
	pubsub_api.SRPCPubSubServiceClient
	cc srpc.Client
}

// NewBifrostAPIClient constructs a new bifrost api client.
func NewBifrostAPIClient(cc srpc.Client) BifrostAPIClient {
	return &bifrostAPIClient{
		SRPCControllerBusServiceClient: bus_api.NewSRPCControllerBusServiceClient(cc),
		SRPCStreamServiceClient:        stream_api.NewSRPCStreamServiceClient(cc),
		SRPCPeerServiceClient:          peer_api.NewSRPCPeerServiceClient(cc),
		SRPCPubSubServiceClient:        pubsub_api.NewSRPCPubSubServiceClient(cc),
		cc:                             cc,
	}
}

// SRPCClient returns the srpc client.
func (c *bifrostAPIClient) SRPCClient() srpc.Client { return c.cc }
