package identity_domain_client

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/identity"
	identity_domain "github.com/aperturerobotics/identity/domain"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// ControllerID is the ID of the controller.
const ControllerID = "identity/client"

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// Domain is the service client backed identity domain.
type Domain struct {
	// b is the bus
	b bus.Bus
	// le is the logger
	le *logrus.Entry
	// conf is the config
	conf *Config

	// identityClient is the aperture identity client
	identityClient *Client
	// peerID is the peer id to use to sign requests.
	peerID peer.ID
}

// NewDomain constructs a new Domain domain controller.
func NewDomain(le *logrus.Entry, b bus.Bus, conf *Config) (*Domain, error) {
	peerID, err := conf.ParsePeerID()
	if err != nil {
		return nil, err
	}
	identityClient, err := NewClient(le, b, peerID, conf.GetClientOpts())
	if err != nil {
		return nil, err
	}
	return &Domain{
		b:    b,
		le:   le,
		conf: conf,

		peerID:         peerID,
		identityClient: identityClient,
	}, nil
}

// Execute executes the domain controller.
// Return nil to exit.
func (a *Domain) Execute(ctx context.Context) error {
	return nil
}

// GetDomainInfo returns the domain info object.
func (a *Domain) GetDomainInfo() *identity_domain.DomainInfo {
	return a.conf.GetDomainInfo().Clone()
}

// LookupPeer looks up the peer id for requests.
func (a *Domain) LookupPeer(ctx context.Context) (peer.Peer, directive.Instance, directive.Reference, error) {
	return peer.GetPeerWithID(ctx, a.b, a.peerID, false, nil)
}

// IdentityLookupEntity implements the IdentityLookupEntity directive.
func (a *Domain) IdentityLookupEntity(
	ctx context.Context,
	dir identity.IdentityLookupEntity,
) (identity.IdentityLookupEntityValue, error) {
	// acquire the configured lookup peer
	peer, _, peerRef, err := a.LookupPeer(ctx)
	if err != nil {
		return nil, err
	}
	defer peerRef.Release()

	peerPriv, err := peer.GetPrivKey(ctx)
	if err != nil {
		return nil, err
	}
	val, err := a.identityClient.LookupEntity(
		ctx,
		peerPriv,
		dir.IdentityLookupEntityDomainID(),
		dir.IdentityLookupEntityID(),
	)
	if err != nil {
		return nil, err
	}
	return identity.NewIdentityLookupEntityValue(
		err,
		err == nil && val == nil,
		val,
	), nil
}

// Close closes any resources for the domain.
func (a *Domain) Close() {}

// _ is a type assertion
var _ identity_domain.Domain = ((*Domain)(nil))
