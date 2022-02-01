package identity_domain_aperture

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/identity"
	identity_domain "github.com/aperturerobotics/identity/domain"
	identity_domain_client "github.com/aperturerobotics/identity/domain/service/client"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the controller.
const ControllerID = "identity/domain/aperture/1"

// ApertureAuth is the aperture auth domain controller.
type ApertureAuth struct {
	// b is the bus
	b bus.Bus
	// le is the logger
	le *logrus.Entry
	// conf is the config
	conf *Config

	// identityClient is the aperture identity client
	identityClient *identity_domain_client.Client
}

// NewApertureAuth constructs a new ApertureAuth domain controller.
func NewApertureAuth(le *logrus.Entry, b bus.Bus, conf *Config) (*ApertureAuth, error) {
	identityClient, err := identity_domain_client.NewClient(le, b, conf.GetIdentityClient())
	if err != nil {
		return nil, err
	}
	return &ApertureAuth{
		b:    b,
		le:   le,
		conf: conf,

		identityClient: identityClient,
	}, nil
}

// Execute executes the domain controller.
// Return nil to exit.
// Returning an error re-constructs the domain controller.
func (a *ApertureAuth) Execute(ctx context.Context) error {
	return a.identityClient.Execute(ctx)
}

// GetDomainInfo returns the domain info object.
func (a *ApertureAuth) GetDomainInfo() *identity_domain.DomainInfo {
	return a.conf.GetDomainInfo().Clone()
}

// IdentityLookupEntity implements the IdentityLookupEntity directive.
func (a *ApertureAuth) IdentityLookupEntity(
	ctx context.Context,
	dir identity.IdentityLookupEntity,
) (identity.IdentityLookupEntityValue, error) {
	// acquire the configured lookup peer
	peer, peerRef, err := a.identityClient.LookupPeer(ctx)
	if err != nil {
		return nil, err
	}
	defer peerRef.Release()

	val, err := a.identityClient.LookupEntity(
		ctx,
		peer.GetPrivKey(),
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
func (a *ApertureAuth) Close() {
	_ = a.identityClient.Close()
}

// _ is a type assertion
var _ identity_domain.Domain = ((*ApertureAuth)(nil))
