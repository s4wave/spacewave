package web_runtime

import "errors"

var (
	// ErrWebDocumentUnavailable is returned if creating WebDocuments is not available.
	ErrWebDocumentUnavailable = errors.New("creating WebDocuments is unavailable")
	// ErrWebDocumentPermanent is returned if WebDocument cannot be closed.
	ErrWebDocumentPermanent = errors.New("WebDocument cannot be closed")

	// ErrEmptyWebRuntimeID is returned if the web runtime ID was empty.
	ErrEmptyWebRuntimeID = errors.New("web runtime id cannot be empty")
)
