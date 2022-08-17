package identity_domain_client

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	stream_srpc_client "github.com/aperturerobotics/bifrost/stream/srpc/client"
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
	// srpcClient is the srpc client instance
	srpcClient stream_srpc_client.Client
}

// NewClient constructs a new client.
func NewClient(
	le *logrus.Entry,
	b bus.Bus,
	peerID peer.ID,
	srpcConf *stream_srpc_client.Config,
) (*Client, error) {
	srv := &Client{
		le:     le,
		b:      b,
		peerID: peerID,
	}
	var err error
	srv.srpcClient, err = stream_srpc_client.NewClient(
		le,
		b,
		srpcConf,
		identity_service.IdentityDomainProtocol,
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
		s.srpcClient,
		lookupPriv,
		domainID, entityID,
	)
}
