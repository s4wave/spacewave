package provider

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/util/labels"
	"github.com/sirupsen/logrus"
)

// ValidateResourceID validates a resource identifier.
func ValidateResourceID(id string) error {
	if id == "" {
		return ErrEmptyResourceID
	}
	if err := labels.ValidateDNSLabel(id); err != nil {
		return errors.Wrap(err, "resource id")
	}
	return nil
}

// Validate validates the resource ref.
func (r *ProviderResourceRef) Validate() error {
	if err := ValidateResourceID(r.GetId()); err != nil {
		return err
	}
	if err := ValidateProviderID(r.GetProviderId()); err != nil {
		return err
	}
	if err := ValidateProviderAccountID(r.GetProviderAccountId()); err != nil {
		return err
	}
	return nil
}

// GetLogger adds debug values to the logger.
func (r *ProviderResourceRef) GetLogger(le *logrus.Entry) *logrus.Entry {
	return le.WithFields(logrus.Fields{
		"resource-id": r.GetId(),
		"provider-id": r.GetProviderId(),
		"account-id":  r.GetProviderAccountId(),
	})
}
