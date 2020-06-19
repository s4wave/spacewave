package identity

import (
	"errors"
	uuid "github.com/satori/go.uuid"
)

// ValidateDomainID checks if a domain ID is valid.
func ValidateDomainID(id string) error {
	if id == "" {
		return errors.New("domain id cannot be empty")
	}
	// TODO additional verification
	return nil
}

// ValidateEntityID checks if a entity ID (username) is valid.
func ValidateEntityID(id string) error {
	if id == "" {
		return errors.New("entity id cannot be empty")
	}
	// TODO additional verification
	return nil
}

// ValidateUUID checks if a uuid is valid.
func ValidateUUID(id string) error {
	// TODO additional verification
	_, err := uuid.FromString(id)
	return err
}

// ValidateDomainUUID checks if the domain-specific UUID is valid.
func ValidateDomainUUID(id string) error {
	return ValidateUUID(id)
}
