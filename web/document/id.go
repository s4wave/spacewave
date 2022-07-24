package web_document

import (
	"github.com/aperturerobotics/bifrost/util/labels"
	"github.com/pkg/errors"
)

// ValidateWebDocumentId validates a document identifier.
func ValidateWebDocumentId(id string) error {
	if id == "" {
		return errors.New("web document id cannot be empty")
	}
	if err := labels.ValidateDNSLabel(id); err != nil {
		return errors.Wrap(err, "web document id")
	}
	return nil
}
