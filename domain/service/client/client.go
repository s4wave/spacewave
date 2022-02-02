package identity_domain_client

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	stream_drpc_client "github.com/aperturerobotics/bifrost/stream/drpc/client"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/identity"
	identity_service "github.com/aperturerobotics/identity/domain/service"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/sirupsen/logrus"
)

// Client is an identity authority client.
type Client struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus

	// peerID is the peer id to use for requests
	peerID peer.ID
	// drpcClient is the drpc client instance
	drpcClient *stream_drpc_client.Client
}

// NewClient constructs a new client.
func NewClient(
	le *logrus.Entry,
	b bus.Bus,
	peerID peer.ID,
	drpcConf *stream_drpc_client.Config,
) (*Client, error) {
	srv := &Client{
		le:     le,
		b:      b,
		peerID: peerID,
	}
	var err error
	srv.drpcClient, err = stream_drpc_client.NewClient(
		le,
		b,
		drpcConf,
	)
	if err != nil {
		return nil, err
	}
	return srv, nil
}

// LookupPeer looks up the peer id for requests.
func (c *Client) LookupPeer(ctx context.Context) (peer.Peer, directive.Reference, error) {
	return peer.GetPeerWithID(ctx, c.b, c.peerID)
}

// LookupEntity requests the Entity corresponding to an entity_id.
//
// returns nil, nil on not found
func (s *Client) LookupEntity(
	ctx context.Context,
	lookupPriv crypto.PrivKey,
	domainID, entityID string,
) (*identity.Entity, error) {
	return identity_service.LookupEntity(
		ctx,
		s.drpcClient,
		lookupPriv,
		domainID, entityID,
	)
}
