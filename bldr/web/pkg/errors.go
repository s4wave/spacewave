package web_pkg

import "errors"

var (
	// ErrEmptyPkgID is returned if the package ID is empty.
	ErrEmptyPkgID = errors.New("package id cannot be empty")
	// ErrInvalidPkgID is returned if the package id is invalid.
	ErrInvalidPkgID = errors.New("package id is invalid")
)
