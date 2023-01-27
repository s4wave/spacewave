package identity_domain_static

import (
	"context"

	"github.com/aperturerobotics/identity"
	identity_domain "github.com/aperturerobotics/identity/domain"
)

// Domain implements the static identity domain.
// Serves identity lookup requests with a static list.
type Domain struct {
	// conf is the configuration
	conf *Config
}

// NewDomain constructs a new Domain.
func NewDomain(c *Config) *Domain {
	return &Domain{conf: c}
}

// GetDomainInfo returns the domain info object.
func (d *Domain) GetDomainInfo() *identity_domain.DomainInfo {
	return d.conf.GetDomainInfo()
}

// Execute executes the domain controller.
func (d *Domain) Execute(ctx context.Context) error { return nil }

// IdentityLookupEntity implements the IdentityLookupEntity directive.
// Return nil, nil if not found.
func (d *Domain) IdentityLookupEntity(
	ctx context.Context,
	dir identity.IdentityLookupEntity,
) (identity.IdentityLookupEntityValue, error) {
	var selEnt *identity.Entity
	for _, ent := range d.conf.GetEntities() {
		if ent.GetEntityId() == dir.IdentityLookupEntityID() && ent.GetDomainId() == dir.IdentityLookupEntityDomainID() {
			if selEnt == nil || selEnt.GetEpoch() < ent.GetEpoch() {
				selEnt = ent
			}
		}
	}
	return identity.NewIdentityLookupEntityValue(nil, selEnt == nil, selEnt.CloneVT()), nil
}

// Close closes any resources for the domain.
func (d *Domain) Close() {
}

// _ is a type assertion
var _ identity_domain.Domain = ((*Domain)(nil))
