package identity_domain

import (
	"context"

	"github.com/aperturerobotics/identity"
)

// Domain is a identity domain implementation.
type Domain interface {
	// GetDomainInfo returns the domain info object.
	GetDomainInfo() *DomainInfo

	// Execute executes the domain controller.
	// Return nil to exit.
	// Returning an error re-constructs the domain controller.
	Execute(ctx context.Context) error

	// IdentityLookupEntity implements the IdentityLookupEntity directive.
	IdentityLookupEntity(
		ctx context.Context,
		dir identity.IdentityLookupEntity,
	) (identity.IdentityLookupEntityValue, error)

	// Close closes any resources for the domain.
	Close()
}

// Handler handles callbacks from the domain
// usually implemented by the domain controller
type Handler interface{}
