package identity_domain_client

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	stream_drpc_client "github.com/aperturerobotics/bifrost/stream/drpc/client"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/identity"
	identity_service "github.com/aperturerobotics/identity/domain/service"
	"github.com/blang/semver"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "aperturerobotics/identity/client/1"

// Client is an identity authority client.
type Client struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus
	// c is the config
	c *Config

	// drpcClient is the drpc client instance
	drpcClient *stream_drpc_client.Client
	// peerID is the peer id to use for requests
	peerID peer.ID
}

// NewClient constructs a new client.
func NewClient(le *logrus.Entry, b bus.Bus, c *Config) (*Client, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	peerID, err := c.ParsePeerID()
	if err != nil {
		return nil, err
	}
	srv := &Client{
		le:     le,
		b:      b,
		c:      c,
		peerID: peerID,
	}
	srv.drpcClient, err = stream_drpc_client.NewClient(
		le,
		b,
		c.GetClientOpts(),
	)
	if err != nil {
		return nil, err
	}
	return srv, nil
}

// GetControllerInfo returns information about the controller.
func (s *Client) GetControllerInfo() controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"identity domain client",
	)
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (s *Client) Execute(ctx context.Context) error {
	return nil
}

// LookupPeer looks up the peer id for requests.
func (s *Client) LookupPeer(ctx context.Context) (peer.Peer, directive.Reference, error) {
	return peer.GetPeerWithID(ctx, s.b, s.peerID)
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

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (s *Client) HandleDirective(ctx context.Context, di directive.Instance) (directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case identity.IdentityLookupEntity:
		return s.resolveLookupEntity(ctx, di, d)
	}
	return nil, nil
}

// DomainIdMatches checks if we will service domain id.
func (s *Client) DomainIdMatches(domainID string) bool {
	ids := s.c.GetDomainIds()
	if len(ids) == 0 {
		return true
	}
	for _, dm := range ids {
		if dm == domainID {
			return true
		}
	}
	return false
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (s *Client) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Client)(nil))
