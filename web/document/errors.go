package web_document

import "errors"

var (
	// ErrEmptyWebDocumentID is returned if the web view id was empty.
	ErrEmptyWebDocumentID = errors.New("empty web document id")
)
