package identity

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/util/labels"
	uuid "github.com/satori/go.uuid"
)

// ValidateDomainID checks if a domain ID is valid.
func ValidateDomainID(id string) error {
	if id == "" {
		return errors.New("domain id cannot be empty")
	}
	if err := labels.ValidateDNSSubdomain(id); err != nil {
		return err
	}
	return nil
}

// ValidateEntityID checks if a entity ID is valid.
func ValidateEntityID(id string) error {
	if id == "" {
		return errors.New("entity id cannot be empty")
	}
	if err := labels.ValidateDNSLabel(id); err != nil {
		return err
	}
	return nil
}

// ValidateUUID checks if a uuid is valid.
func ValidateUUID(id string) error {
	_, err := uuid.FromString(id)
	return err
}

// ValidateDomainUUID checks if the domain-specific UUID is valid.
func ValidateDomainUUID(id string) error {
	return ValidateUUID(id)
}
