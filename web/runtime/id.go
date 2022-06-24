package web_runtime

import (
	"github.com/aperturerobotics/bifrost/util/labels"
	"github.com/pkg/errors"
)

// ValidateRuntimeId validates a runtime identifier.
func ValidateRuntimeId(id string) error {
	if id == "" {
		return errors.New("web runtime id cannot be empty")
	}
	if err := labels.ValidateDNSLabel(id); err != nil {
		return errors.Wrap(err, "web runtime id")
	}
	return nil
}
