package identity_domain_service

import (
	"github.com/s4wave/spacewave/identity"
)

// Validate checks the identifier.
func (i *EntityLookupIdentifier) Validate() error {
	if err := identity.ValidateDomainID(i.GetDomainId()); err != nil {
		return err
	}
	if err := identity.ValidateEntityID(i.GetEntityId()); err != nil {
		return err
	}
	return nil
}
