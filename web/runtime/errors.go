package web_runtime

import "errors"

var (
	// ErrWebDocumentUnavailable is returned if creating WebDocuments is not available.
	ErrWebDocumentUnavailable = errors.New("creating WebDocuments is unavailable")
	// ErrWebDocumentPermanent is returned if WebDocument cannot be closed.
	ErrWebDocumentPermanent = errors.New("WebDocument cannot be closed")
)
